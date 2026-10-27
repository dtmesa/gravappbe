package store

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"golang.org/x/sync/errgroup"

	"github.com/dtmesa/gravappbe/internal/dynamo"
	"github.com/dtmesa/gravappbe/internal/model"
)

// Cascading deletes replace Prisma's onDelete: Cascade. DynamoDB has no
// referential cascade, so the fan-out is explicit.
//
// Every method removes the entity itself -- and releases its name claim --
// BEFORE touching its children, so a concurrent read or create sees it as gone
// immediately and consistently, rather than a parent that still exists with
// some children already removed.
//
// Known, accepted limitation: this is not atomic. A mid-cascade failure leaves
// orphaned children, and parent-first ordering means a retried DELETE 404s on
// the parent rather than resuming. Immediate read consistency was judged more
// valuable than resumability; orphans are invisible and harmless.

// DeleteExerciseSession deletes one exercise session and its sets.
func (s *Store) DeleteExerciseSession(ctx context.Context, workoutSessionID, id int) error {
	if err := s.deleteKey(ctx, s.db.T.ExerciseSessions, exerciseSessionKey(workoutSessionID, id)); err != nil {
		return err
	}

	return s.deleteSets(ctx, []int{id})
}

// DeleteWorkoutSession deletes a session, its exercise sessions and their sets.
func (s *Store) DeleteWorkoutSession(ctx context.Context, userID, id int) error {
	if err := s.deleteKey(ctx, s.db.T.WorkoutSessions, sessionKey(userID, id)); err != nil {
		return err
	}

	children, err := s.exerciseSessionKeys(ctx, id)
	if err != nil {
		return err
	}

	return s.deleteExerciseSessions(ctx, children)
}

// DeleteExercise deletes an exercise and every exercise session recorded
// against it, wherever it was logged.
func (s *Store) DeleteExercise(ctx context.Context, ex model.Exercise) error {
	if err := s.releaseExerciseName(ctx, ex.WorkoutID, ex.Name); err != nil {
		return err
	}

	if err := s.deleteKey(ctx, s.db.T.Exercises, exerciseKey(ex.WorkoutID, ex.ID)); err != nil {
		return err
	}

	children, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.db.T.ExerciseSessions),
		IndexName:                 aws.String(dynamo.IndexExerciseSessionsByExercise),
		KeyConditionExpression:    aws.String("exerciseId = :e"),
		ProjectionExpression:      aws.String("workoutSessionId, id"),
		ExpressionAttributeValues: dynamo.Item{":e": dynamo.N(ex.ID)},
	})
	if err != nil {
		return err
	}

	return s.deleteExerciseSessions(ctx, children)
}

// DeleteWorkout deletes a workout, its sessions, its exercises and everything
// beneath them.
func (s *Store) DeleteWorkout(ctx context.Context, w model.Workout) error {
	if err := s.releaseWorkoutName(ctx, w.UserID, w.Name); err != nil {
		return err
	}

	if err := s.deleteKey(ctx, s.db.T.Workouts, workoutKey(w.UserID, w.ID)); err != nil {
		return err
	}

	// The workoutId-date-index GSI existed only for this lookup. A user's
	// sessions are one partition, so filtering it costs far less than a GSI
	// that taxed every session write.
	sessions, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.db.T.WorkoutSessions),
		KeyConditionExpression:    aws.String("userId = :u"),
		FilterExpression:          aws.String("workoutId = :w"),
		ProjectionExpression:      aws.String("id"),
		ExpressionAttributeValues: dynamo.Item{":u": dynamo.N(w.UserID), ":w": dynamo.N(w.ID)},
	})
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if err := s.DeleteWorkoutSession(ctx, w.UserID, dynamo.Int(session, "id")); err != nil {
			return err
		}
	}

	exercises, err := s.Exercises(ctx, w.ID)
	if err != nil {
		return err
	}

	// An exercise can be logged into a session of a different workout, so each
	// is swept by exerciseId rather than relying on the session pass above.
	for _, ex := range exercises {
		if err := s.DeleteExercise(ctx, ex); err != nil {
			return err
		}
	}

	return nil
}

// DeleteUserData deletes an account's data. The caller removes the user row
// (and its claims) first, so the account stops working immediately.
func (s *Store) DeleteUserData(ctx context.Context, userID int) error {
	workouts, err := s.Workouts(ctx, userID)
	if err != nil {
		return err
	}

	for _, w := range workouts {
		if err := s.DeleteWorkout(ctx, w); err != nil {
			return err
		}
	}

	// Sessions whose workout was already gone would be missed above.
	sessions, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(s.db.T.WorkoutSessions),
		KeyConditionExpression:    aws.String("userId = :u"),
		ProjectionExpression:      aws.String("id"),
		ExpressionAttributeValues: dynamo.Item{":u": dynamo.N(userID)},
	})
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if err := s.DeleteWorkoutSession(ctx, userID, dynamo.Int(session, "id")); err != nil {
			return err
		}
	}

	return nil
}

// deleteExerciseSessions deletes exercise sessions (by key) and their sets.
func (s *Store) deleteExerciseSessions(ctx context.Context, keys []dynamo.Item) error {
	ids := make([]int, len(keys))
	for i, key := range keys {
		ids[i] = dynamo.Int(key, "id")
	}

	if err := s.db.BatchDelete(ctx, s.db.T.ExerciseSessions, keys); err != nil {
		return err
	}

	return s.deleteSets(ctx, ids)
}

// deleteSets deletes every set under the given exercise sessions.
func (s *Store) deleteSets(ctx context.Context, exerciseSessionIDs []int) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(fanOut)

	for _, id := range exerciseSessionIDs {
		g.Go(func() error {
			keys, err := s.db.QueryAll(ctx, &dynamodb.QueryInput{
				TableName:                 aws.String(s.db.T.SetSessions),
				KeyConditionExpression:    aws.String("exerciseSessionId = :e"),
				ProjectionExpression:      aws.String("exerciseSessionId, id"),
				ExpressionAttributeValues: dynamo.Item{":e": dynamo.N(id)},
			})
			if err != nil {
				return err
			}

			return s.db.BatchDelete(ctx, s.db.T.SetSessions, keys)
		})
	}

	return g.Wait()
}
