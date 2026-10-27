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

// Exercises are partitioned by workoutId, and everything is scoped by the
// workout named in the URL. (The Express version located an exercise by id
// across all of the caller's workouts, so /workouts/1/exercises/99 could
// return an exercise from workout 2; the C# port tightened that, and so does
// this.) Name uniqueness per workout is a conditional put on ExerciseNames.

var (
	errExerciseNotFound  = httpx.NotFound("Exercise", "EXERCISE_NOT_FOUND")
	errExerciseNameTaken = httpx.New(http.StatusConflict, "EXERCISE_NAME_TAKEN", "Exercise name already in use")
)

func exerciseKey(workoutID, id int) dynamo.Item { return dynamo.Key("workoutId", workoutID, "id", id) }

func (s *Store) Exercise(ctx context.Context, workoutID, id int) (*model.Exercise, error) {
	var ex model.Exercise

	found, err := s.get(ctx, s.db.T.Exercises, exerciseKey(workoutID, id), true, &ex)
	if !found || err != nil {
		return nil, err
	}

	return &ex, nil
}

// RequireExercise returns the exercise or 404 EXERCISE_NOT_FOUND.
func (s *Store) RequireExercise(ctx context.Context, workoutID, id int) (model.Exercise, error) {
	ex, err := s.Exercise(ctx, workoutID, id)
	if err != nil {
		return model.Exercise{}, err
	}

	if ex == nil {
		return model.Exercise{}, errExerciseNotFound
	}

	return *ex, nil
}

// Exercises lists a workout's exercises by (order, id).
func (s *Store) Exercises(ctx context.Context, workoutID int) ([]model.Exercise, error) {
	items, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.db.T.Exercises),
		KeyConditionExpression:    aws.String("workoutId = :w"),
		ExpressionAttributeValues: dynamo.Item{":w": dynamo.N(workoutID)},
	})
	if err != nil {
		return nil, err
	}

	exercises, err := unmarshalAll[model.Exercise](items)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(exercises, func(a, b model.Exercise) int {
		return cmp.Or(cmp.Compare(a.Order, b.Order), cmp.Compare(a.ID, b.ID))
	})

	return exercises, nil
}

// CreateExercise appends after the current highest order, as the client
// expects (it appends locally before refetching). The original computed
// (last?.order ?? 0) + 1, so the first exercise is order 1, not 0; that offset
// is invisible to the client and kept.
func (s *Store) CreateExercise(ctx context.Context, workoutID int, name string) (model.Exercise, error) {
	items, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.db.T.Exercises),
		KeyConditionExpression:    aws.String("workoutId = :w"),
		ProjectionExpression:      aws.String("#o"),
		ExpressionAttributeNames:  map[string]string{"#o": "order"},
		ExpressionAttributeValues: dynamo.Item{":w": dynamo.N(workoutID)},
	})
	if err != nil {
		return model.Exercise{}, err
	}

	highest := 0
	for _, item := range items {
		highest = max(highest, dynamo.Int(item, "order"))
	}

	id, err := s.nextID(ctx, entityExercise)
	if err != nil {
		return model.Exercise{}, err
	}

	ex := model.Exercise{ID: id, Name: name, WorkoutID: workoutID, Order: highest + 1, CreatedAt: s.stamp()}

	item, err := attributevalue.MarshalMap(ex)
	if err != nil {
		return model.Exercise{}, err
	}

	_, err = s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			s.claimExerciseName(workoutID, name, id),
			{Put: &types.Put{TableName: aws.String(s.db.T.Exercises), Item: item}},
		},
	})
	if dynamo.AnyConditionFailed(err) {
		return model.Exercise{}, errExerciseNameTaken
	}

	return ex, err
}

// UpdateExercise mirrors UpdateWorkout: rename is a transaction, anything else
// one conditional update, and a missing exercise is always EXERCISE_NOT_FOUND.
func (s *Store) UpdateExercise(ctx context.Context, workoutID, id int, p valid.ExercisePatch) (model.Exercise, error) {
	var current model.Exercise

	rename := p.Name != nil

	if rename {
		var err error
		if current, err = s.RequireExercise(ctx, workoutID, id); err != nil {
			return model.Exercise{}, err
		}

		rename = *p.Name != current.Name
	}

	e := newSetExpr()
	if p.Description != nil {
		e.set("description", dynamo.S(*p.Description))
	}

	if p.Order != nil {
		e.set("order", dynamo.N(*p.Order))
	}

	for attr, v := range map[string]*bool{
		"isWeight": p.IsWeight, "isDuration": p.IsDuration, "isDistance": p.IsDistance, "isReps": p.IsReps,
	} {
		if v != nil {
			e.set(attr, dynamo.Bool(*v))
		}
	}

	if !rename {
		if e.empty() {
			return current, nil
		}

		out, err := s.db.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName:                 aws.String(s.db.T.Exercises),
			Key:                       exerciseKey(workoutID, id),
			UpdateExpression:          e.expression(),
			ConditionExpression:       aws.String("attribute_exists(id)"),
			ExpressionAttributeNames:  e.names,
			ExpressionAttributeValues: e.values,
			ReturnValues:              types.ReturnValueAllNew,
		})
		if dynamo.IsConditionFailed(err) {
			return model.Exercise{}, errExerciseNotFound
		}

		if err != nil {
			return model.Exercise{}, err
		}

		var ex model.Exercise

		return ex, attributevalue.UnmarshalMap(out.Attributes, &ex)
	}

	e.set("name", dynamo.S(*p.Name))

	_, err := s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			s.claimExerciseName(workoutID, *p.Name, id),
			{Delete: &types.Delete{
				TableName: aws.String(s.db.T.ExerciseNames),
				Key:       dynamo.Item{"workoutId": dynamo.N(workoutID), "name": dynamo.S(current.Name)},
			}},
			{Update: &types.Update{
				TableName:                 aws.String(s.db.T.Exercises),
				Key:                       exerciseKey(workoutID, id),
				UpdateExpression:          e.expression(),
				ConditionExpression:       aws.String("attribute_exists(id)"),
				ExpressionAttributeNames:  e.names,
				ExpressionAttributeValues: e.values,
			}},
		},
	})

	switch {
	case dynamo.ConditionFailedAt(err, 0):
		return model.Exercise{}, errExerciseNameTaken
	case dynamo.ConditionFailedAt(err, 2):
		return model.Exercise{}, errExerciseNotFound
	case err != nil:
		return model.Exercise{}, err
	}

	// The transaction returns no values, so re-read for the full row.
	return s.RequireExercise(ctx, workoutID, id)
}

// ReorderExercises sets order = index for each id, atomically.
func (s *Store) ReorderExercises(ctx context.Context, workoutID int, ids []int) error {
	return s.reorder(ctx, s.db.T.Exercises, "workoutId", workoutID, ids, errExerciseNotFound)
}

func (s *Store) claimExerciseName(workoutID int, name string, exerciseID int) types.TransactWriteItem {
	return types.TransactWriteItem{Put: &types.Put{
		TableName:                aws.String(s.db.T.ExerciseNames),
		Item:                     dynamo.Item{"workoutId": dynamo.N(workoutID), "name": dynamo.S(name), "exerciseId": dynamo.N(exerciseID)},
		ConditionExpression:      aws.String("attribute_not_exists(#n)"),
		ExpressionAttributeNames: map[string]string{"#n": "name"},
	}}
}

func (s *Store) releaseExerciseName(ctx context.Context, workoutID int, name string) error {
	return s.deleteKey(ctx, s.db.T.ExerciseNames, dynamo.Item{"workoutId": dynamo.N(workoutID), "name": dynamo.S(name)})
}
