package store

import (
	"context"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/dtmesa/gravappbe/internal/dynamo"
	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/model"
	"github.com/dtmesa/gravappbe/internal/valid"
)

// Sets are partitioned by exerciseSessionId with id as the sort key, which
// gives the creation order the client relies on (it treats the last element as
// the previous set) at no cost.
//
// Every set write also updates the parent's aggregates in the same
// transaction, conditioned on the set still holding the values the delta was
// computed from. So aggregates cannot drift from a lost write, and a racing
// write simply retries.

var (
	errSetNotFound = httpx.NotFound("Set session", "SETSESSION_NOT_FOUND")
	errSetConflict = httpx.New(http.StatusConflict, "CONFLICT", "Set was modified concurrently")
)

func setKey(exerciseSessionID, id int) dynamo.Item {
	return dynamo.Key("exerciseSessionId", exerciseSessionID, "id", id)
}

// Sets lists an exercise session's sets in creation order.
func (s *Store) Sets(ctx context.Context, exerciseSessionID int) ([]model.SetSession, error) {
	return s.sets(ctx, exerciseSessionID, false)
}

func (s *Store) sets(ctx context.Context, exerciseSessionID int, consistent bool) ([]model.SetSession, error) {
	items, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.db.T.SetSessions),
		KeyConditionExpression:    aws.String("exerciseSessionId = :e"),
		ExpressionAttributeValues: dynamo.Item{":e": dynamo.N(exerciseSessionID)},
		ScanIndexForward:          aws.Bool(true),
		ConsistentRead:            aws.Bool(consistent),
	})
	if err != nil {
		return nil, err
	}

	return unmarshalAll[model.SetSession](items)
}

func (s *Store) Set(ctx context.Context, exerciseSessionID, id int) (*model.SetSession, error) {
	var set model.SetSession

	found, err := s.get(ctx, s.db.T.SetSessions, setKey(exerciseSessionID, id), true, &set)
	if !found || err != nil {
		return nil, err
	}

	return &set, nil
}

// RequireSet returns the set or 404 SETSESSION_NOT_FOUND.
func (s *Store) RequireSet(ctx context.Context, exerciseSessionID, id int) (model.SetSession, error) {
	set, err := s.Set(ctx, exerciseSessionID, id)
	if err != nil {
		return model.SetSession{}, err
	}

	if set == nil {
		return model.SetSession{}, errSetNotFound
	}

	return *set, nil
}

// CreateSet seeds 0 for each metric the exercise tracks and null for the rest.
// The caller must have run EnsureAggregates on es.
func (s *Store) CreateSet(ctx context.Context, es model.ExerciseSession, ex model.Exercise) (model.SetSession, error) {
	id, err := s.nextID(ctx, entitySetSession)
	if err != nil {
		return model.SetSession{}, err
	}

	set := model.SetSession{ID: id, ExerciseSessionID: es.ID, CreatedAt: s.stamp()}

	if ex.IsWeight {
		set.Weight = new(float64)
	}

	if ex.IsReps {
		set.Reps = new(int)
	}

	if ex.IsDuration {
		set.Duration = new(int)
	}

	if ex.IsDistance {
		set.Distance = new(float64)
	}

	item, err := attributevalue.MarshalMap(set)
	if err != nil {
		return model.SetSession{}, err
	}

	_, err = s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{
				TableName:           aws.String(s.db.T.SetSessions),
				Item:                item,
				ConditionExpression: aws.String("attribute_not_exists(id)"),
			}},
			s.addToParent(es, 1, contribution(set)),
		},
	})
	if dynamo.ConditionFailedAt(err, 1) {
		return model.SetSession{}, errExerciseSessionNotFound
	}

	return set, err
}

// UpdateSet applies a patch to cur. The caller must have run EnsureAggregates
// on es.
func (s *Store) UpdateSet(ctx context.Context, es model.ExerciseSession, cur model.SetSession, p valid.SetPatch) (model.SetSession, error) {
	for range 3 {
		next := cur
		if p.Weight != nil {
			next.Weight = p.Weight
		}

		if p.Reps != nil {
			next.Reps = p.Reps
		}

		if p.Duration != nil {
			next.Duration = p.Duration
		}

		if p.Distance != nil {
			next.Distance = p.Distance
		}

		cond, names, values := unchanged(cur)

		e := newSetExpr()
		e.names, e.values = names, values

		for _, metric := range model.Metrics {
			if v := next.Value(metric); v != nil && !sameValue(cur.Value(metric), v) {
				e.set(metric, metricValue(next, metric))
			}
		}

		if e.empty() {
			return cur, nil
		}

		_, err := s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
			TransactItems: []types.TransactWriteItem{
				{Update: &types.Update{
					TableName:                 aws.String(s.db.T.SetSessions),
					Key:                       setKey(cur.ExerciseSessionID, cur.ID),
					UpdateExpression:          e.expression(),
					ConditionExpression:       aws.String(cond),
					ExpressionAttributeNames:  e.names,
					ExpressionAttributeValues: e.values,
				}},
				s.addToParent(es, 0, minus(contribution(next), contribution(cur))),
			},
		})

		switch {
		case dynamo.ConditionFailedAt(err, 0):
			// Deleted or changed since it was read: re-read and retry.
			fresh, err := s.Set(ctx, cur.ExerciseSessionID, cur.ID)
			if err != nil {
				return model.SetSession{}, err
			}

			if fresh == nil {
				return model.SetSession{}, errSetNotFound
			}

			cur = *fresh

			continue
		case dynamo.ConditionFailedAt(err, 1):
			return model.SetSession{}, errExerciseSessionNotFound
		case err != nil:
			return model.SetSession{}, err
		}

		return next, nil
	}

	return model.SetSession{}, errSetConflict
}

// DeleteSet removes cur. The caller must have run EnsureAggregates on es.
func (s *Store) DeleteSet(ctx context.Context, es model.ExerciseSession, cur model.SetSession) error {
	for range 3 {
		cond, names, values := unchanged(cur)

		del := &types.Delete{
			TableName:                aws.String(s.db.T.SetSessions),
			Key:                      setKey(cur.ExerciseSessionID, cur.ID),
			ConditionExpression:      aws.String(cond),
			ExpressionAttributeNames: names,
		}

		// DynamoDB rejects an empty values map; an all-null set has none.
		if len(values) > 0 {
			del.ExpressionAttributeValues = values
		}

		_, err := s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
			TransactItems: []types.TransactWriteItem{
				{Delete: del},
				s.addToParent(es, -1, minus(model.Aggregates{}, contribution(cur))),
			},
		})

		switch {
		case dynamo.ConditionFailedAt(err, 0):
			fresh, err := s.Set(ctx, cur.ExerciseSessionID, cur.ID)
			if err != nil {
				return err
			}

			if fresh == nil {
				return errSetNotFound
			}

			cur = *fresh

			continue
		case dynamo.ConditionFailedAt(err, 1):
			return errExerciseSessionNotFound
		default:
			return err
		}
	}

	return errSetConflict
}

// addToParent is the aggregate half of a set write. It requires the parent to
// carry aggregates already, so a delta can never land on a row that has not
// been backfilled -- and, as a side effect, can never recreate a deleted one.
func (s *Store) addToParent(es model.ExerciseSession, setCountDelta int, d model.Aggregates) types.TransactWriteItem {
	return types.TransactWriteItem{Update: &types.Update{
		TableName: aws.String(s.db.T.ExerciseSessions),
		Key:       exerciseSessionKey(es.WorkoutSessionID, es.ID),
		UpdateExpression: aws.String("ADD setCount :c, weightSum :ws, weightCount :wc, repsSum :rs, repsCount :rc, " +
			"durationSum :ds, durationCount :dc, distanceSum :xs, distanceCount :xc"),
		ConditionExpression: aws.String("attribute_exists(setCount)"),
		ExpressionAttributeValues: dynamo.Item{
			":c":  dynamo.N(setCountDelta),
			":ws": dynamo.F(d.WeightSum), ":wc": dynamo.N(d.WeightCount),
			":rs": dynamo.F(d.RepsSum), ":rc": dynamo.N(d.RepsCount),
			":ds": dynamo.F(d.DurationSum), ":dc": dynamo.N(d.DurationCount),
			":xs": dynamo.F(d.DistanceSum), ":xc": dynamo.N(d.DistanceCount),
		},
	}}
}

// contribution is what one set adds to its parent's sums and counts.
func contribution(set model.SetSession) model.Aggregates {
	var a model.Aggregates
	a.Add(set, 1)

	return a
}

func minus(a, b model.Aggregates) model.Aggregates {
	return model.Aggregates{
		WeightSum: a.WeightSum - b.WeightSum, WeightCount: a.WeightCount - b.WeightCount,
		RepsSum: a.RepsSum - b.RepsSum, RepsCount: a.RepsCount - b.RepsCount,
		DurationSum: a.DurationSum - b.DurationSum, DurationCount: a.DurationCount - b.DurationCount,
		DistanceSum: a.DistanceSum - b.DistanceSum, DistanceCount: a.DistanceCount - b.DistanceCount,
	}
}

// unchanged builds a condition that the set still exists and still holds the
// metric values in cur, so the delta computed from cur is exact.
func unchanged(cur model.SetSession) (string, map[string]string, dynamo.Item) {
	parts := []string{"attribute_exists(id)"}
	names := map[string]string{}
	values := dynamo.Item{}

	for _, metric := range model.Metrics {
		name := "#c" + metric

		names[name] = metric

		if cur.Value(metric) == nil {
			parts = append(parts, "attribute_not_exists("+name+")")
		} else {
			parts = append(parts, name+" = :c"+metric)
			values[":c"+metric] = metricValue(cur, metric)
		}
	}

	return strings.Join(parts, " AND "), names, values
}

func metricValue(set model.SetSession, metric string) types.AttributeValue {
	switch metric {
	case model.MetricWeight:
		return dynamo.F(*set.Weight)
	case model.MetricDistance:
		return dynamo.F(*set.Distance)
	case model.MetricReps:
		return dynamo.N(*set.Reps)
	default:
		return dynamo.N(*set.Duration)
	}
}

func sameValue(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}
