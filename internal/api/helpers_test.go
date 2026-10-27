package api_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/dtmesa/gravappbe/internal/model"
	"github.com/dtmesa/gravappbe/internal/testutil"
)

func createWorkout(t *testing.T, c *testutil.Client, name string) model.Workout {
	t.Helper()

	var w model.Workout
	c.Expect(201, "POST", "/workouts", map[string]string{"name": name}).JSON(t, &w)

	return w
}

func exercisesPath(w model.Workout) string { return fmt.Sprintf("/workouts/%d/exercises", w.ID) }

func createExercise(t *testing.T, c *testutil.Client, w model.Workout, name string, flags ...string) model.Exercise {
	t.Helper()

	var ex model.Exercise
	c.Expect(201, "POST", exercisesPath(w), map[string]string{"name": name}).JSON(t, &ex)

	for _, flag := range flags {
		c.Expect(200, "PATCH", fmt.Sprintf("%s/%d/%s", exercisesPath(w), ex.ID, flag), map[string]bool{flag: true}).JSON(t, &ex)
	}

	return ex
}

func sessionsPath(w model.Workout) string { return fmt.Sprintf("/workouts/%d/sessions", w.ID) }

func createSession(t *testing.T, c *testutil.Client, w model.Workout) model.WorkoutSession {
	t.Helper()

	var ws model.WorkoutSession
	c.Expect(201, "POST", sessionsPath(w), nil).JSON(t, &ws)

	return ws
}

func createSessionAt(t *testing.T, c *testutil.Client, w model.Workout, date string) model.WorkoutSession {
	t.Helper()

	var ws model.WorkoutSession
	c.Expect(201, "POST", sessionsPath(w), map[string]string{"date": date}).JSON(t, &ws)

	return ws
}

func exerciseSessionsPath(w model.Workout, ws model.WorkoutSession) string {
	return fmt.Sprintf("/workouts/%d/sessions/%d/exerciseSessions", w.ID, ws.ID)
}

func createExerciseSession(t *testing.T, c *testutil.Client, w model.Workout, ws model.WorkoutSession, ex model.Exercise) model.ExerciseSession {
	t.Helper()

	var es model.ExerciseSession
	c.Expect(201, "POST", exerciseSessionsPath(w, ws), map[string]int{"exerciseId": ex.ID}).JSON(t, &es)

	return es
}

func setsPath(w model.Workout, ws model.WorkoutSession, es model.ExerciseSession) string {
	return fmt.Sprintf("%s/%d/setSessions", exerciseSessionsPath(w, ws), es.ID)
}

// addSet creates a set and patches the given metrics onto it.
func addSet(t *testing.T, c *testutil.Client, w model.Workout, ws model.WorkoutSession, es model.ExerciseSession, metrics map[string]any) model.SetSession {
	t.Helper()

	var set model.SetSession
	c.Expect(201, "POST", setsPath(w, ws, es), nil).JSON(t, &set)

	for field, value := range metrics {
		c.Expect(200, "PATCH", fmt.Sprintf("%s/%d/%s", setsPath(w, ws, es), set.ID, field), map[string]any{field: value}).JSON(t, &set)
	}

	return set
}

// assertGone checks, directly against the tables, that a workout and the
// session tree under it were removed.
func assertGone(t *testing.T, env *testutil.Env, w model.Workout, ws model.WorkoutSession, es model.ExerciseSession) {
	t.Helper()

	ctx := context.Background()

	if got, err := env.Store.Workout(ctx, w.UserID, w.ID); err != nil || got != nil {
		t.Errorf("workout survived: %+v %v", got, err)
	}

	if got, err := env.Store.WorkoutSession(ctx, w.UserID, ws.ID); err != nil || got != nil {
		t.Errorf("session survived: %+v %v", got, err)
	}

	if got, err := env.Store.ExerciseSession(ctx, ws.ID, es.ID); err != nil || got != nil {
		t.Errorf("exercise session survived: %+v %v", got, err)
	}

	if sets, err := env.Store.Sets(ctx, es.ID); err != nil || len(sets) != 0 {
		t.Errorf("sets survived: %+v %v", sets, err)
	}
}
