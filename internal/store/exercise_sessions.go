package store

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"golang.org/x/sync/errgroup"

	"github.com/dtmesa/gravappbe/internal/dynamo"
	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/jsonx"
	"github.com/dtmesa/gravappbe/internal/model"
)

// Exercise sessions are partitioned by workoutSessionId. Each carries a copy of
// its parent session's userId, workoutId and date (the exerciseId-sessionDate
// GSI that /averages and /previous-set-count read depends on them) and the
// running set aggregates.

var errExerciseSessionNotFound = httpx.NotFound("Exercise session", "EXERCISESESSION_NOT_FOUND")

func exerciseSessionKey(workoutSessionID, id int) dynamo.Item {
	return dynamo.Key("workoutSessionId", workoutSessionID, "id", id)
}

func (s *Store) ExerciseSession(ctx context.Context, workoutSessionID, id int) (*model.ExerciseSession, error) {
	var es model.ExerciseSession

	found, err := s.get(ctx, s.db.T.ExerciseSessions, exerciseSessionKey(workoutSessionID, id), true, &es)
	if !found || err != nil {
		return nil, err
	}

	return &es, nil
}

// RequireExerciseSession returns the exercise session or 404
// EXERCISESESSION_NOT_FOUND.
func (s *Store) RequireExerciseSession(ctx context.Context, workoutSessionID, id int) (model.ExerciseSession, error) {
	es, err := s.ExerciseSession(ctx, workoutSessionID, id)
	if err != nil {
		return model.ExerciseSession{}, err
	}

	if es == nil {
		return model.ExerciseSession{}, errExerciseSessionNotFound
	}

	return *es, nil
}

// ExerciseSessions lists a session's exercise sessions in id order.
func (s *Store) ExerciseSessions(ctx context.Context, workoutSessionID int) ([]model.ExerciseSession, error) {
	items, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.db.T.ExerciseSessions),
		KeyConditionExpression:    aws.String("workoutSessionId = :w"),
		ExpressionAttributeValues: dynamo.Item{":w": dynamo.N(workoutSessionID)},
	})
	if err != nil {
		return nil, err
	}

	return unmarshalAll[model.ExerciseSession](items)
}

func (s *Store) exerciseSessionKeys(ctx context.Context, workoutSessionID int) ([]dynamo.Item, error) {
	return s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.db.T.ExerciseSessions),
		KeyConditionExpression:    aws.String("workoutSessionId = :w"),
		ProjectionExpression:      aws.String("workoutSessionId, id"),
		ExpressionAttributeValues: dynamo.Item{":w": dynamo.N(workoutSessionID)},
	})
}

// CreateExerciseSessions creates one exercise session per exercise id under
// ws. One id block, and a single transaction when there is more than one, so
// starting a workout is one call rather than the client's N parallel creates.
func (s *Store) CreateExerciseSessions(ctx context.Context, ws model.WorkoutSession, exerciseIDs []int) ([]model.ExerciseSession, error) {
	first, err := s.nextIDs(ctx, entityExerciseSession, len(exerciseIDs))
	if err != nil {
		return nil, err
	}

	created := make([]model.ExerciseSession, len(exerciseIDs))
	puts := make([]types.TransactWriteItem, len(exerciseIDs))

	for i, exerciseID := range exerciseIDs {
		count := 0

		created[i] = model.ExerciseSession{
			ID: first + i,
			// The Prisma create never set order, so it was always 0.
			Order:            0,
			WorkoutSessionID: ws.ID,
			ExerciseID:       exerciseID,
			CreatedAt:        s.stamp(),
			Sets:             []model.SetSession{},
			UserID:           ws.UserID,
			WorkoutID:        ws.WorkoutID,
			SessionDate:      ws.Date,
			Aggregates:       model.Aggregates{SetCount: &count},
		}

		item, err := attributevalue.MarshalMap(created[i])
		if err != nil {
			return nil, err
		}

		puts[i] = types.TransactWriteItem{Put: &types.Put{TableName: aws.String(s.db.T.ExerciseSessions), Item: item}}
	}

	if len(puts) == 1 {
		_, err = s.db.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: puts[0].Put.TableName, Item: puts[0].Put.Item})
	} else {
		_, err = s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: puts})
	}

	return created, err
}

// ExerciseSessionsForExercise backs /averages. since is nil for the lifetime
// variant; excludeWorkoutSessionID drops the session in progress.
func (s *Store) ExerciseSessionsForExercise(ctx context.Context, exerciseID, userID int, since *time.Time, excludeWorkoutSessionID int) ([]model.ExerciseSession, error) {
	values := dynamo.Item{
		":e": dynamo.N(exerciseID),
		":u": dynamo.N(userID),
		":x": dynamo.N(excludeWorkoutSessionID),
	}

	condition := "exerciseId = :e"
	if since != nil {
		condition += " AND sessionDate >= :since"
		values[":since"] = dynamo.S(jsonx.New(*since).String())
	}

	items, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.db.T.ExerciseSessions),
		IndexName:                 aws.String(dynamo.IndexExerciseSessionsByExercise),
		KeyConditionExpression:    aws.String(condition),
		FilterExpression:          aws.String("userId = :u AND workoutSessionId <> :x"),
		ExpressionAttributeValues: values,
	})
	if err != nil {
		return nil, err
	}

	return unmarshalAll[model.ExerciseSession](items)
}

// PreviousExerciseSession returns the most recent logging of this exercise in
// the same workout, other than the given one -- ordered by the parent session's
// date, as Prisma's orderBy: { workoutSession: { date } } was.
//
// The C# version read every page before taking the first match, so it cost
// O(history) on every call. This reads small pages newest-first and stops at
// the first match. (The filter is applied after Limit, hence pages rather than
// Limit 1.)
func (s *Store) PreviousExerciseSession(ctx context.Context, exerciseID, workoutID, userID, excludeID int) (*model.ExerciseSession, error) {
	in := &dynamodb.QueryInput{
		TableName:              aws.String(s.db.T.ExerciseSessions),
		IndexName:              aws.String(dynamo.IndexExerciseSessionsByExercise),
		KeyConditionExpression: aws.String("exerciseId = :e"),
		FilterExpression:       aws.String("workoutId = :w AND userId = :u AND id <> :x"),
		ExpressionAttributeValues: dynamo.Item{
			":e": dynamo.N(exerciseID), ":w": dynamo.N(workoutID), ":u": dynamo.N(userID), ":x": dynamo.N(excludeID),
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(25),
	}

	for {
		out, err := s.db.Client.Query(ctx, in)
		if err != nil {
			return nil, err
		}

		if len(out.Items) > 0 {
			var es model.ExerciseSession
			return &es, attributevalue.UnmarshalMap(out.Items[0], &es)
		}

		if len(out.LastEvaluatedKey) == 0 {
			return nil, nil
		}

		in.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

// EnsureAggregates backfills the set aggregates on a row written before they
// existed, or repairs ones a lost write left impossible, by recomputing from
// the sets. Every set write calls this before applying its own delta, which
// is what keeps a delta from ever landing on an un-backfilled row.
//
// The update is conditional on the row being exactly as read, so a concurrent
// backfill or set write wins rather than being overwritten; in that case the
// row is re-read and its aggregates used.
func (s *Store) EnsureAggregates(ctx context.Context, es *model.ExerciseSession) error {
	if es.HasAggregates() && es.Consistent() {
		return nil
	}

	sets, err := s.sets(ctx, es.ID, true)
	if err != nil {
		return err
	}

	agg := model.AggregatesOf(sets)

	condition := "attribute_exists(id) AND attribute_not_exists(setCount)"
	values := dynamo.Item{}

	if es.HasAggregates() {
		condition = "attribute_exists(id) AND setCount = :prev"
		values[":prev"] = dynamo.N(*es.SetCount)
	}

	e := newSetExpr()
	e.set("setCount", dynamo.N(*agg.SetCount))
	e.set("weightSum", dynamo.F(agg.WeightSum))
	e.set("weightCount", dynamo.N(agg.WeightCount))
	e.set("repsSum", dynamo.F(agg.RepsSum))
	e.set("repsCount", dynamo.N(agg.RepsCount))
	e.set("durationSum", dynamo.F(agg.DurationSum))
	e.set("durationCount", dynamo.N(agg.DurationCount))
	e.set("distanceSum", dynamo.F(agg.DistanceSum))
	e.set("distanceCount", dynamo.N(agg.DistanceCount))

	for k, v := range values {
		e.values[k] = v
	}

	_, err = s.db.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.db.T.ExerciseSessions),
		Key:                       exerciseSessionKey(es.WorkoutSessionID, es.ID),
		UpdateExpression:          e.expression(),
		ConditionExpression:       aws.String(condition),
		ExpressionAttributeNames:  e.names,
		ExpressionAttributeValues: e.values,
	})

	if dynamo.IsConditionFailed(err) {
		fresh, err := s.ExerciseSession(ctx, es.WorkoutSessionID, es.ID)
		if err != nil {
			return err
		}

		if fresh == nil {
			return errExerciseSessionNotFound
		}

		if !fresh.HasAggregates() {
			return errExerciseSessionNotFound
		}

		es.Aggregates = fresh.Aggregates

		return nil
	}

	if err != nil {
		return err
	}

	es.Aggregates = agg

	return nil
}

// EnsureAllAggregates backfills a batch in parallel, for /averages. A row
// deleted mid-flight counts as having no sets rather than failing the request.
func (s *Store) EnsureAllAggregates(ctx context.Context, sessions []model.ExerciseSession) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(fanOut)

	for i := range sessions {
		if sessions[i].HasAggregates() && sessions[i].Consistent() {
			continue
		}

		g.Go(func() error {
			err := s.EnsureAggregates(ctx, &sessions[i])
			if err == errExerciseSessionNotFound {
				sessions[i].Aggregates = model.Aggregates{}
				return nil
			}

			return err
		})
	}

	return g.Wait()
}
