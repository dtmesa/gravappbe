package store

import (
	"cmp"
	"context"
	"net/http"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/dtmesa/gravappbe/internal/dynamo"
	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/model"
	"github.com/dtmesa/gravappbe/internal/valid"
)

// Workouts are partitioned by userId, so an ownership check is a plain
// GetItem. Name uniqueness per user is a conditional put on WorkoutNames.

var (
	errWorkoutNotFound  = httpx.NotFound("Workout", "WORKOUT_NOT_FOUND")
	errWorkoutNameTaken = httpx.New(http.StatusConflict, "WORKOUT_NAME_TAKEN", "Workout name already in use")
)

func workoutKey(userID, id int) dynamo.Item { return dynamo.Key("userId", userID, "id", id) }

func (s *Store) Workout(ctx context.Context, userID, id int) (*model.Workout, error) {
	var w model.Workout

	found, err := s.get(ctx, s.db.T.Workouts, workoutKey(userID, id), true, &w)
	if !found || err != nil {
		return nil, err
	}

	return &w, nil
}

// RequireWorkout returns the workout or 404 WORKOUT_NOT_FOUND.
func (s *Store) RequireWorkout(ctx context.Context, userID, id int) (model.Workout, error) {
	w, err := s.Workout(ctx, userID, id)
	if err != nil {
		return model.Workout{}, err
	}

	if w == nil {
		return model.Workout{}, errWorkoutNotFound
	}

	return *w, nil
}

// Workouts lists a user's workouts by (order, id). The client renders the
// array as returned and never sorts it. The list is small enough to sort in
// memory, which is why the order-index LSI is no longer read.
func (s *Store) Workouts(ctx context.Context, userID int) ([]model.Workout, error) {
	items, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.db.T.Workouts),
		KeyConditionExpression:    aws.String("userId = :u"),
		ExpressionAttributeValues: dynamo.Item{":u": dynamo.N(userID)},
	})
	if err != nil {
		return nil, err
	}

	workouts, err := unmarshalAll[model.Workout](items)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(workouts, func(a, b model.Workout) int {
		return cmp.Or(cmp.Compare(a.Order, b.Order), cmp.Compare(a.ID, b.ID))
	})

	return workouts, nil
}

// CreateWorkout places the new workout first, as the client expects -- it
// prepends locally before refetching.
//
// It takes order = min(existing) - 1 instead of the original "order 0 and
// shift every sibling down by one". The shift rewrote every sibling in one
// transaction, which capped an account at 98 workouts (TransactWriteItems
// allows 100 actions), lost updates when two creates raced (the sibling list
// was read outside the transaction), and resurrected a concurrently deleted
// workout as a stub (the shifts were unconditional). Orders drift negative,
// which is harmless: the client only ever compares them, and its own
// drag-reorder renumbers the list 0..n-1. Concurrent creates may tie; ties sort
// by id.
func (s *Store) CreateWorkout(ctx context.Context, userID int, name string) (model.Workout, error) {
	items, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.db.T.Workouts),
		KeyConditionExpression:    aws.String("userId = :u"),
		ProjectionExpression:      aws.String("#o"),
		ExpressionAttributeNames:  map[string]string{"#o": "order"},
		ExpressionAttributeValues: dynamo.Item{":u": dynamo.N(userID)},
	})
	if err != nil {
		return model.Workout{}, err
	}

	order := 0
	for i, item := range items {
		if o := dynamo.Int(item, "order"); i == 0 || o-1 < order {
			order = o - 1
		}
	}

	id, err := s.nextID(ctx, entityWorkout)
	if err != nil {
		return model.Workout{}, err
	}

	w := model.Workout{ID: id, Name: name, Order: order, UserID: userID, CreatedAt: s.stamp()}

	item, err := attributevalue.MarshalMap(w)
	if err != nil {
		return model.Workout{}, err
	}

	_, err = s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			s.claimWorkoutName(userID, name, id),
			{Put: &types.Put{TableName: aws.String(s.db.T.Workouts), Item: item}},
		},
	})
	if dynamo.AnyConditionFailed(err) {
		return model.Workout{}, errWorkoutNameTaken
	}

	return w, err
}

// UpdateWorkout applies a patch. A rename has to release the old name claim
// and take the new one atomically, so it runs as a transaction; any other
// change is a single conditional update. A missing workout is always
// WORKOUT_NOT_FOUND -- the C# service returned a bare NOT_FOUND for some fields
// and WORKOUT_NOT_FOUND for others, depending on the code path.
func (s *Store) UpdateWorkout(ctx context.Context, userID, id int, p valid.WorkoutPatch) (model.Workout, error) {
	var current model.Workout

	rename := p.Name != nil

	if rename {
		var err error
		if current, err = s.RequireWorkout(ctx, userID, id); err != nil {
			return model.Workout{}, err
		}

		// Renaming to the current name is a no-op rather than a conflict
		// against the workout's own claim.
		rename = *p.Name != current.Name
	}

	e := newSetExpr()
	if p.Description != nil {
		e.set("description", dynamo.S(*p.Description))
	}

	if p.Order != nil {
		e.set("order", dynamo.N(*p.Order))
	}

	if !rename {
		if e.empty() {
			return current, nil
		}

		out, err := s.db.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName:                 aws.String(s.db.T.Workouts),
			Key:                       workoutKey(userID, id),
			UpdateExpression:          e.expression(),
			ConditionExpression:       aws.String("attribute_exists(id)"),
			ExpressionAttributeNames:  e.names,
			ExpressionAttributeValues: e.values,
			ReturnValues:              types.ReturnValueAllNew,
		})
		if dynamo.IsConditionFailed(err) {
			return model.Workout{}, errWorkoutNotFound
		}

		if err != nil {
			return model.Workout{}, err
		}

		var w model.Workout

		return w, attributevalue.UnmarshalMap(out.Attributes, &w)
	}

	e.set("name", dynamo.S(*p.Name))

	_, err := s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			s.claimWorkoutName(userID, *p.Name, id),
			{Delete: &types.Delete{
				TableName: aws.String(s.db.T.WorkoutNames),
				Key:       dynamo.Item{"userId": dynamo.N(userID), "name": dynamo.S(current.Name)},
			}},
			{Update: &types.Update{
				TableName:                 aws.String(s.db.T.Workouts),
				Key:                       workoutKey(userID, id),
				UpdateExpression:          e.expression(),
				ConditionExpression:       aws.String("attribute_exists(id)"),
				ExpressionAttributeNames:  e.names,
				ExpressionAttributeValues: e.values,
			}},
		},
	})

	switch {
	case dynamo.ConditionFailedAt(err, 0):
		return model.Workout{}, errWorkoutNameTaken
	case dynamo.ConditionFailedAt(err, 2):
		return model.Workout{}, errWorkoutNotFound
	case err != nil:
		return model.Workout{}, err
	}

	// The transaction returns no values, so re-read for the full row.
	return s.RequireWorkout(ctx, userID, id)
}

// ReorderWorkouts sets order = index for each id, atomically. It replaces the
// client's N parallel PATCHes with one call, whenever the client adopts it.
func (s *Store) ReorderWorkouts(ctx context.Context, userID int, ids []int) error {
	return s.reorder(ctx, s.db.T.Workouts, "userId", userID, ids, errWorkoutNotFound)
}

func (s *Store) reorder(ctx context.Context, table, partition string, parentID int, ids []int, notFound error) error {
	items := make([]types.TransactWriteItem, len(ids))

	for i, id := range ids {
		items[i] = types.TransactWriteItem{Update: &types.Update{
			TableName:                 aws.String(table),
			Key:                       dynamo.Key(partition, parentID, "id", id),
			UpdateExpression:          aws.String("SET #o = :o"),
			ConditionExpression:       aws.String("attribute_exists(id)"),
			ExpressionAttributeNames:  map[string]string{"#o": "order"},
			ExpressionAttributeValues: dynamo.Item{":o": dynamo.N(i)},
		}}
	}

	_, err := s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if dynamo.AnyConditionFailed(err) {
		return notFound
	}

	return err
}

func (s *Store) claimWorkoutName(userID int, name string, workoutID int) types.TransactWriteItem {
	return types.TransactWriteItem{Put: &types.Put{
		TableName:                aws.String(s.db.T.WorkoutNames),
		Item:                     dynamo.Item{"userId": dynamo.N(userID), "name": dynamo.S(name), "workoutId": dynamo.N(workoutID)},
		ConditionExpression:      aws.String("attribute_not_exists(#n)"),
		ExpressionAttributeNames: map[string]string{"#n": "name"},
	}}
}

// releaseWorkoutName frees a workout's name so a new workout can reuse it.
func (s *Store) releaseWorkoutName(ctx context.Context, userID int, name string) error {
	return s.deleteKey(ctx, s.db.T.WorkoutNames, dynamo.Item{"userId": dynamo.N(userID), "name": dynamo.S(name)})
}
