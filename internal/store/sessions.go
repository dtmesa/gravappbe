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

// Workout sessions are partitioned by userId, with the date-index LSI serving
// the history calendar's month window.

var errSessionNotFound = httpx.NotFound("Workout session", "WORKOUTSESSION_NOT_FOUND")

func sessionKey(userID, id int) dynamo.Item { return dynamo.Key("userId", userID, "id", id) }

func (s *Store) WorkoutSession(ctx context.Context, userID, id int) (*model.WorkoutSession, error) {
	var ws model.WorkoutSession

	found, err := s.get(ctx, s.db.T.WorkoutSessions, sessionKey(userID, id), true, &ws)
	if !found || err != nil {
		return nil, err
	}

	return &ws, nil
}

// RequireWorkoutSession returns the session or 404 WORKOUTSESSION_NOT_FOUND.
func (s *Store) RequireWorkoutSession(ctx context.Context, userID, id int) (model.WorkoutSession, error) {
	ws, err := s.WorkoutSession(ctx, userID, id)
	if err != nil {
		return model.WorkoutSession{}, err
	}

	if ws == nil {
		return model.WorkoutSession{}, errSessionNotFound
	}

	return *ws, nil
}

// CreateWorkoutSession uses the given date, or now.
func (s *Store) CreateWorkoutSession(ctx context.Context, userID, workoutID int, date *jsonx.Timestamp) (model.WorkoutSession, error) {
	id, err := s.nextID(ctx, entityWorkoutSession)
	if err != nil {
		return model.WorkoutSession{}, err
	}

	ws := model.WorkoutSession{ID: id, UserID: userID, WorkoutID: workoutID, Date: s.stamp(), CreatedAt: s.stamp()}
	if date != nil {
		ws.Date = jsonx.New(date.Time)
	}

	return ws, s.put(ctx, s.db.T.WorkoutSessions, ws)
}

// SessionsBetween returns sessions dated in [start, end), ascending.
func (s *Store) SessionsBetween(ctx context.Context, userID int, start, end time.Time) ([]model.WorkoutSession, error) {
	items, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                aws.String(s.db.T.WorkoutSessions),
		IndexName:                aws.String(dynamo.IndexSessionsByDate),
		KeyConditionExpression:   aws.String("userId = :u AND #d BETWEEN :start AND :end"),
		ExpressionAttributeNames: map[string]string{"#d": "date"},
		ExpressionAttributeValues: dynamo.Item{
			":u":     dynamo.N(userID),
			":start": dynamo.S(jsonx.New(start).String()),
			// BETWEEN is inclusive and the original filter was `lt: end`, so
			// step back a millisecond. This only works because every stored
			// date has exactly three fractional digits.
			":end": dynamo.S(jsonx.New(end.Add(-time.Millisecond)).String()),
		},
		ScanIndexForward: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}

	return unmarshalAll[model.WorkoutSession](items)
}

// UpdateSessionDate moves a session, then moves the denormalized copy of its
// date on every child exercise session, which the averages index sorts on.
// The two steps are not atomic: a failure between them leaves the children's
// sessionDate stale, which affects only averages ordering.
func (s *Store) UpdateSessionDate(ctx context.Context, userID, id int, date jsonx.Timestamp) (model.WorkoutSession, error) {
	out, err := s.db.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.db.T.WorkoutSessions),
		Key:                       sessionKey(userID, id),
		UpdateExpression:          aws.String("SET #d = :d"),
		ConditionExpression:       aws.String("attribute_exists(id)"),
		ExpressionAttributeNames:  map[string]string{"#d": "date"},
		ExpressionAttributeValues: dynamo.Item{":d": dynamo.S(date.String())},
		ReturnValues:              types.ReturnValueAllNew,
	})
	if dynamo.IsConditionFailed(err) {
		return model.WorkoutSession{}, errSessionNotFound
	}

	if err != nil {
		return model.WorkoutSession{}, err
	}

	var ws model.WorkoutSession
	if err := attributevalue.UnmarshalMap(out.Attributes, &ws); err != nil {
		return model.WorkoutSession{}, err
	}

	return ws, s.syncSessionDate(ctx, id, date)
}

func (s *Store) syncSessionDate(ctx context.Context, workoutSessionID int, date jsonx.Timestamp) error {
	keys, err := s.exerciseSessionKeys(ctx, workoutSessionID)
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(fanOut)

	for _, key := range keys {
		g.Go(func() error {
			_, err := s.db.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName:                 aws.String(s.db.T.ExerciseSessions),
				Key:                       key,
				UpdateExpression:          aws.String("SET sessionDate = :d"),
				ConditionExpression:       aws.String("attribute_exists(id)"),
				ExpressionAttributeValues: dynamo.Item{":d": dynamo.S(date.String())},
			})

			// A child deleted mid-flight is fine; the condition only stops the
			// update from recreating it as a stub.
			if dynamo.IsConditionFailed(err) {
				return nil
			}

			return err
		})
	}

	return g.Wait()
}
