package api_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dtmesa/gravappbe/internal/model"
	"github.com/dtmesa/gravappbe/internal/testutil"
)

func TestCreateWorkoutPrependsAndListIsSorted(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()

	first := createWorkout(t, c, "Push")

	if first.Order != 0 || first.Description != nil {
		t.Errorf("first workout = %+v", first)
	}

	res := c.Expect(201, "POST", "/workouts", map[string]string{"name": "Pull"})
	if !strings.Contains(string(res.Body), `"description":null`) {
		t.Errorf("description not an explicit null: %s", res.Body)
	}

	var second model.Workout
	res.JSON(t, &second)

	third := createWorkout(t, c, "Legs")

	// The client prepends a new workout locally, so the server must list it
	// first too -- without rewriting every sibling to make room.
	if !(third.Order < second.Order && second.Order < first.Order) {
		t.Errorf("orders = %d, %d, %d; newest should be lowest", third.Order, second.Order, first.Order)
	}

	assertWorkoutOrder(t, c, third.ID, second.ID, first.ID)

	// Siblings were not rewritten.
	var got model.Workout
	c.Expect(200, "GET", fmt.Sprintf("/workouts/%d", first.ID), nil).JSON(t, &got)

	if got.Order != 0 {
		t.Errorf("first workout's order changed to %d", got.Order)
	}
}

func TestWorkoutNamesAreUniquePerUser(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	other, _ := env.User()

	w := createWorkout(t, c, "Push")

	if res := c.Do("POST", "/workouts", map[string]string{"name": "Push"}); res.Status != 409 || res.Code() != "WORKOUT_NAME_TAKEN" {
		t.Errorf("duplicate = %s", res)
	}

	// Scoped per user.
	createWorkout(t, other, "Push")

	// A rename releases the old name.
	c.Expect(200, "PATCH", fmt.Sprintf("/workouts/%d/name", w.ID), map[string]string{"name": "Push A"})
	createWorkout(t, c, "Push")

	if res := c.Do("PATCH", fmt.Sprintf("/workouts/%d/name", w.ID), map[string]string{"name": "Push"}); res.Code() != "WORKOUT_NAME_TAKEN" {
		t.Errorf("rename into a taken name = %s", res)
	}

	// Renaming to its own name is a no-op, not a conflict.
	c.Expect(200, "PATCH", fmt.Sprintf("/workouts/%d/name", w.ID), map[string]string{"name": "Push A"})

	// Delete releases it too.
	c.Expect(204, "DELETE", fmt.Sprintf("/workouts/%d", w.ID), nil)
	createWorkout(t, c, "Push A")
}

func TestWorkoutOwnershipAndNotFound(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	intruder, _ := env.User()

	w := createWorkout(t, c, "Mine")

	for _, req := range []struct{ method, path string }{
		{"GET", fmt.Sprintf("/workouts/%d", w.ID)},
		{"DELETE", fmt.Sprintf("/workouts/%d", w.ID)},
		{"GET", exercisesPath(w)},
	} {
		if res := intruder.Do(req.method, req.path, nil); res.Status != 404 || res.Code() != "WORKOUT_NOT_FOUND" {
			t.Errorf("%s %s as another user = %s", req.method, req.path, res)
		}
	}

	// A missing workout is WORKOUT_NOT_FOUND on every path now, including the
	// generic field patch that used to answer a bare NOT_FOUND.
	for _, field := range []string{"order", "description", "name"} {
		body := map[string]any{field: "x"}
		if field == "order" {
			body[field] = 1
		}

		if res := intruder.Do("PATCH", fmt.Sprintf("/workouts/%d/%s", w.ID, field), body); res.Code() != "WORKOUT_NOT_FOUND" {
			t.Errorf("patch %s on another user's workout = %s", field, res)
		}
	}

	// Non-numeric and non-positive ids, as the {id:int} constraint behaved.
	if res := c.Do("GET", "/workouts/abc", nil); res.Status != 404 {
		t.Errorf("non-numeric id = %s", res)
	}

	if res := c.Do("GET", "/workouts/0", nil); res.Status != 400 {
		t.Errorf("zero id = %s", res)
	}
}

func TestWorkoutFieldPatch(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")
	path := fmt.Sprintf("/workouts/%d", w.ID)

	var got model.Workout
	c.Expect(200, "PATCH", path+"/description", map[string]string{"description": "Chest day"}).JSON(t, &got)

	if got.Description == nil || *got.Description != "Chest day" || got.Name != "Push" {
		t.Errorf("after description patch = %+v", got)
	}

	c.Expect(200, "PATCH", path+"/order", map[string]int{"order": 5}).JSON(t, &got)

	if got.Order != 5 {
		t.Errorf("order = %d", got.Order)
	}

	for field, body := range map[string]any{
		"userId": map[string]int{"userId": 99},
		"order":  map[string]any{"order": -1},
		"name":   map[string]string{"name": " padded"},
	} {
		if res := c.Do("PATCH", path+"/"+field, body); res.Status != 400 || res.Code() != "VALIDATION_ERROR" {
			t.Errorf("patch %s %v = %s", field, body, res)
		}
	}

	// Additive sparse form: several fields at once, rename included.
	c.Expect(200, "PATCH", path, map[string]any{"name": "Push B", "description": "x", "order": 2}).JSON(t, &got)

	if got.Name != "Push B" || *got.Description != "x" || got.Order != 2 {
		t.Errorf("sparse patch = %+v", got)
	}
}

func TestReorderWorkouts(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()

	a := createWorkout(t, c, "A")
	b := createWorkout(t, c, "B")
	d := createWorkout(t, c, "D")

	// The live client reorders with one PATCH per item, order = index.
	for i, id := range []int{a.ID, d.ID, b.ID} {
		c.Expect(200, "PATCH", fmt.Sprintf("/workouts/%d/order", id), map[string]int{"order": i})
	}

	assertWorkoutOrder(t, c, a.ID, d.ID, b.ID)

	// The additive bulk form does it in one atomic call.
	c.Expect(204, "PUT", "/workouts/order", map[string][]int{"ids": {b.ID, a.ID, d.ID}})
	assertWorkoutOrder(t, c, b.ID, a.ID, d.ID)

	if res := c.Do("PUT", "/workouts/order", map[string][]int{"ids": {b.ID, 999999999}}); res.Code() != "WORKOUT_NOT_FOUND" {
		t.Errorf("reorder with an unknown id = %s", res)
	}

	// A failed reorder changed nothing.
	assertWorkoutOrder(t, c, b.ID, a.ID, d.ID)
}

func TestExercises(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")
	other := createWorkout(t, c, "Pull")

	bench := createExercise(t, c, w, "Bench")
	fly := createExercise(t, c, w, "Fly")

	// Appended: the client appends new exercises locally. The first is order 1,
	// the original's (last?.order ?? 0) + 1.
	if bench.Order != 1 || fly.Order != 2 || bench.WorkoutID != w.ID {
		t.Errorf("orders = %d, %d", bench.Order, fly.Order)
	}

	if bench.IsWeight || bench.IsReps || bench.IsDuration || bench.IsDistance {
		t.Errorf("new exercise has flags set: %+v", bench)
	}

	var list []model.Exercise
	c.Expect(200, "GET", exercisesPath(w), nil).JSON(t, &list)

	if len(list) != 2 || list[0].ID != bench.ID || list[1].ID != fly.ID {
		t.Errorf("list = %+v", list)
	}

	if res := c.Do("POST", exercisesPath(w), map[string]string{"name": "Bench"}); res.Status != 409 || res.Code() != "EXERCISE_NAME_TAKEN" {
		t.Errorf("duplicate = %s", res)
	}

	// Unique per workout, not per user.
	createExercise(t, c, other, "Bench")

	// Exercises are resolved within the workout in the URL.
	if res := c.Do("GET", fmt.Sprintf("%s/%d", exercisesPath(other), fly.ID), nil); res.Status != 404 || res.Code() != "EXERCISE_NOT_FOUND" {
		t.Errorf("exercise via the wrong workout = %s", res)
	}

	var got model.Exercise
	c.Expect(200, "PATCH", fmt.Sprintf("%s/%d/isWeight", exercisesPath(w), bench.ID), map[string]bool{"isWeight": true}).JSON(t, &got)

	if !got.IsWeight || got.Name != "Bench" {
		t.Errorf("flag patch = %+v", got)
	}

	c.Expect(200, "PATCH", fmt.Sprintf("%s/%d", exercisesPath(w), bench.ID), map[string]any{"name": "Incline", "isReps": true}).JSON(t, &got)

	if got.Name != "Incline" || !got.IsReps || !got.IsWeight {
		t.Errorf("sparse patch = %+v", got)
	}

	if res := c.Do("PATCH", fmt.Sprintf("%s/%d/order", exercisesPath(w), 999999999), map[string]int{"order": 1}); res.Code() != "EXERCISE_NOT_FOUND" {
		t.Errorf("patch missing exercise = %s", res)
	}

	c.Expect(204, "PUT", exercisesPath(w)+"/order", map[string][]int{"ids": {fly.ID, bench.ID}})
	c.Expect(200, "GET", exercisesPath(w), nil).JSON(t, &list)

	if list[0].ID != fly.ID {
		t.Errorf("after reorder = %+v", list)
	}
}

func TestDeleteWorkoutCascades(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()

	w := createWorkout(t, c, "Push")
	ex := createExercise(t, c, w, "Bench", "isWeight")
	ws := createSession(t, c, w)
	es := createExerciseSession(t, c, w, ws, ex)
	addSet(t, c, w, ws, es, map[string]any{"weight": 100})

	// An exercise from this workout logged into another workout's session
	// must go too -- the cascade sweeps by exercise, not just by session.
	w2 := createWorkout(t, c, "Pull")
	ex2 := createExercise(t, c, w2, "Row")
	ws2 := createSession(t, c, w2)
	es2 := createExerciseSession(t, c, w2, ws2, ex2)

	c.Expect(204, "DELETE", fmt.Sprintf("/workouts/%d", w.ID), nil)

	assertGone(t, env, w, ws, es)

	if res := c.Do("GET", fmt.Sprintf("/workouts/%d", w.ID), nil); res.Code() != "WORKOUT_NOT_FOUND" {
		t.Errorf("deleted workout = %s", res)
	}

	// The unrelated workout is untouched.
	c.Expect(200, "GET", fmt.Sprintf("%s/%d", exerciseSessionsPath(w2, ws2), es2.ID), nil)
}

func TestDeleteExerciseCascades(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()

	w := createWorkout(t, c, "Push")
	ex := createExercise(t, c, w, "Bench", "isWeight")
	keep := createExercise(t, c, w, "Fly", "isWeight")
	ws := createSession(t, c, w)
	es := createExerciseSession(t, c, w, ws, ex)
	kept := createExerciseSession(t, c, w, ws, keep)
	addSet(t, c, w, ws, es, nil)

	c.Expect(204, "DELETE", fmt.Sprintf("%s/%d", exercisesPath(w), ex.ID), nil)

	var list []model.ExerciseSession
	c.Expect(200, "GET", exerciseSessionsPath(w, ws), nil).JSON(t, &list)

	if len(list) != 1 || list[0].ID != kept.ID {
		t.Errorf("exercise sessions after delete = %+v", list)
	}

	// Its name is reusable.
	createExercise(t, c, w, "Bench")
}

func assertWorkoutOrder(t *testing.T, c *testutil.Client, ids ...int) {
	t.Helper()

	var list []model.Workout
	c.Expect(200, "GET", "/workouts", nil).JSON(t, &list)

	got := make([]int, len(list))
	for i, w := range list {
		got[i] = w.ID
	}

	if fmt.Sprint(got) != fmt.Sprint(ids) {
		t.Errorf("workout order = %v, want %v", got, ids)
	}
}
