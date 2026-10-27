package store

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/dtmesa/gravappbe/internal/dynamo"
	"github.com/dtmesa/gravappbe/internal/model"
)

// History assembles the /history/sessions response for [start, end). Prisma
// served this from one query with nested includes; multi-table DynamoDB needs
// a fan-out instead: the month's sessions, then every exercise session, then
// every set list, then two batch reads for workout and exercise names. Each
// fan-out stage is bounded, unlike the C# Task.WhenAll.
//
// Names are read live, so history reflects later renames, as it always has.
func (s *Store) History(ctx context.Context, userID int, start, end time.Time) ([]model.HistorySession, error) {
	sessions, err := s.SessionsBetween(ctx, userID, start, end)
	if err != nil || len(sessions) == 0 {
		return []model.HistorySession{}, err
	}

	children := make([][]model.ExerciseSession, len(sessions))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(fanOut)

	for i, ws := range sessions {
		g.Go(func() (err error) {
			children[i], err = s.ExerciseSessions(gctx, ws.ID)
			return err
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Flatten so every set query runs in one bounded stage.
	type ref struct{ session, child int }

	var refs []ref
	for i := range children {
		for j := range children[i] {
			refs = append(refs, ref{i, j})
		}
	}

	sets := make([][]model.SetSession, len(refs))

	g, gctx = errgroup.WithContext(ctx)
	g.SetLimit(fanOut)

	for k, r := range refs {
		g.Go(func() (err error) {
			sets[k], err = s.Sets(gctx, children[r.session][r.child].ID)
			return err
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	workoutNames, exerciseNames, err := s.names(ctx, userID, sessions, children)
	if err != nil {
		return nil, err
	}

	out := make([]model.HistorySession, len(sessions))
	k := 0

	for i, ws := range sessions {
		exercises := make([]model.HistoryExerciseSession, len(children[i]))

		for j, es := range children[i] {
			exercises[j] = model.HistoryExerciseSession{
				ID:               es.ID,
				Order:            es.Order,
				WorkoutSessionID: es.WorkoutSessionID,
				ExerciseID:       es.ExerciseID,
				CreatedAt:        es.CreatedAt,
				Exercise:         model.NamedRef{Name: exerciseNames[es.ExerciseID]},
				Sets:             sets[k],
			}
			k++
		}

		out[i] = model.HistorySession{
			ID:        ws.ID,
			Date:      ws.Date,
			UserID:    ws.UserID,
			WorkoutID: ws.WorkoutID,
			CreatedAt: ws.CreatedAt,
			Workout:   model.NamedRef{Name: workoutNames[ws.WorkoutID]},
			Exercises: exercises,
		}
	}

	return out, nil
}

// names resolves workout and exercise names with one batch read each. A name
// that no longer resolves comes back as "", as before.
func (s *Store) names(ctx context.Context, userID int, sessions []model.WorkoutSession, children [][]model.ExerciseSession) (workouts, exercises map[int]string, err error) {
	workoutKeys := map[int]dynamo.Item{}
	for _, ws := range sessions {
		workoutKeys[ws.WorkoutID] = workoutKey(userID, ws.WorkoutID)
	}

	type exerciseRef struct{ workout, exercise int }

	exerciseKeys := map[exerciseRef]dynamo.Item{}
	for _, list := range children {
		for _, es := range list {
			exerciseKeys[exerciseRef{es.WorkoutID, es.ExerciseID}] = exerciseKey(es.WorkoutID, es.ExerciseID)
		}
	}

	workouts, err = s.nameMap(ctx, s.db.T.Workouts, values(workoutKeys))
	if err != nil {
		return nil, nil, err
	}

	exercises, err = s.nameMap(ctx, s.db.T.Exercises, values(exerciseKeys))

	return workouts, exercises, err
}

func (s *Store) nameMap(ctx context.Context, table string, keys []dynamo.Item) (map[int]string, error) {
	names := map[int]string{}
	if len(keys) == 0 {
		return names, nil
	}

	items, err := s.db.BatchGet(ctx, table, keys)
	if err != nil {
		return nil, err
	}

	for _, item := range items {
		var row struct {
			ID   int    `dynamodbav:"id"`
			Name string `dynamodbav:"name"`
		}

		if err := unmarshalItem(item, &row); err != nil {
			return nil, err
		}

		names[row.ID] = row.Name
	}

	return names, nil
}

func values[K comparable](m map[K]dynamo.Item) []dynamo.Item {
	out := make([]dynamo.Item, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}

	return out
}
