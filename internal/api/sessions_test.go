package api_test

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/dtmesa/gravappbe/internal/jsonx"
	"github.com/dtmesa/gravappbe/internal/model"
	"github.com/dtmesa/gravappbe/internal/testutil"
)

func TestWorkoutSessions(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")

	// No body means "now", and the workout is nested in the response.
	res := c.Expect(201, "POST", sessionsPath(w), nil)
	if !strings.Contains(string(res.Body), `"date":"2031-06-15T12:00:00.250Z"`) || !strings.Contains(string(res.Body), `"workout":{`) {
		t.Errorf("create = %s", res.Body)
	}

	var ws model.WorkoutSession
	res.JSON(t, &ws)

	dated := createSessionAt(t, c, w, "2031-06-01T08:30:00.000Z")
	if dated.Date.String() != "2031-06-01T08:30:00.000Z" {
		t.Errorf("dated session = %s", dated.Date)
	}

	// Reads ignore the workout in the URL, as the original did.
	var got model.WorkoutSession
	c.Expect(200, "GET", fmt.Sprintf("/workouts/%d/sessions/%d", w.ID+1000, ws.ID), nil).JSON(t, &got)

	if got.ID != ws.ID || got.Workout == nil || got.Workout.Name != "Push" {
		t.Errorf("get = %+v", got)
	}

	// ...but it must still be an integer, as the {workoutId:int} constraint
	// required.
	if res := c.Do("GET", fmt.Sprintf("/workouts/undefined/sessions/%d", ws.ID), nil); res.Status != 404 {
		t.Errorf("non-numeric workout id = %s", res)
	}

	other, _ := env.User()
	if res := other.Do("GET", fmt.Sprintf("%s/%d", sessionsPath(w), ws.ID), nil); res.Code() != "WORKOUTSESSION_NOT_FOUND" {
		t.Errorf("another user's session = %s", res)
	}

	if res := other.Do("POST", sessionsPath(w), nil); res.Code() != "WORKOUT_NOT_FOUND" {
		t.Errorf("session on another user's workout = %s", res)
	}
}

func TestPatchSessionDateMovesChildren(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")
	ex := createExercise(t, c, w, "Bench")
	ws := createSession(t, c, w)
	es := createExerciseSession(t, c, w, ws, ex)
	path := fmt.Sprintf("%s/%d", sessionsPath(w), ws.ID)

	var got model.WorkoutSession
	c.Expect(200, "PATCH", path+"/date", map[string]string{"date": "2031-05-01T07:00:00.123456Z"}).JSON(t, &got)

	// Truncated to the millisecond, on update as well as create.
	if got.Date.String() != "2031-05-01T07:00:00.123Z" {
		t.Errorf("patched date = %s", got.Date)
	}

	// The denormalized copy that the averages index sorts on moved too.
	row, err := env.Store.ExerciseSession(context.Background(), ws.ID, es.ID)
	if err != nil || row == nil || row.SessionDate.String() != "2031-05-01T07:00:00.123Z" {
		t.Errorf("child sessionDate = %+v, %v", row, err)
	}

	if res := c.Do("PATCH", path+"/date", map[string]string{}); res.Status != 400 || res.Code() != "INVALID_BODY" {
		t.Errorf("missing date = %s", res)
	}

	if res := c.Do("PATCH", path+"/date", map[string]string{"date": "soon"}); res.Code() != "VALIDATION_ERROR" {
		t.Errorf("bad date = %s", res)
	}

	if res := c.Do("PATCH", path+"/workoutId", map[string]int{"workoutId": 1}); res.Code() != "VALIDATION_ERROR" {
		t.Errorf("unsupported field = %s", res)
	}
}

func TestExerciseSessions(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")
	w2 := createWorkout(t, c, "Pull")
	ex := createExercise(t, c, w, "Bench")
	ex2 := createExercise(t, c, w2, "Row")
	ws := createSession(t, c, w)
	ws2 := createSession(t, c, w2)

	res := c.Expect(201, "POST", exerciseSessionsPath(w, ws), map[string]int{"exerciseId": ex.ID})
	if !strings.Contains(string(res.Body), `"sets":[]`) || !strings.Contains(string(res.Body), `"order":0`) {
		t.Errorf("create = %s", res.Body)
	}

	var es model.ExerciseSession
	res.JSON(t, &es)

	// The session must belong to the workout in the URL -- previously this
	// let workout A's exercise into workout B's session.
	if res := c.Do("POST", exerciseSessionsPath(w, ws2), map[string]int{"exerciseId": ex.ID}); res.Code() != "WORKOUTSESSION_NOT_FOUND" {
		t.Errorf("mismatched workout/session = %s", res)
	}

	if res := c.Do("POST", exerciseSessionsPath(w, ws), map[string]int{"exerciseId": ex2.ID}); res.Code() != "EXERCISE_NOT_FOUND" {
		t.Errorf("exercise from another workout = %s", res)
	}

	if res := c.Do("POST", exerciseSessionsPath(w, ws), map[string]int{"exerciseId": 0}); res.Code() != "VALIDATION_ERROR" {
		t.Errorf("zero exerciseId = %s", res)
	}

	addSet(t, c, w, ws, es, nil)

	var list []model.ExerciseSession
	c.Expect(200, "GET", exerciseSessionsPath(w, ws), nil).JSON(t, &list)

	if len(list) != 1 || len(list[0].Sets) != 1 {
		t.Errorf("list = %+v", list)
	}

	// A fresh session's list is [], never null or a 404 -- the app bootstraps
	// a workout from it.
	empty := createSession(t, c, w)
	if res := c.Expect(200, "GET", exerciseSessionsPath(w, empty), nil); string(res.Body) != "[]" {
		t.Errorf("empty list = %s", res.Body)
	}

	intruder, _ := env.User()
	if res := intruder.Do("GET", fmt.Sprintf("%s/%d", exerciseSessionsPath(w, ws), es.ID), nil); res.Status != 404 {
		t.Errorf("another user's exercise session = %s", res)
	}

	c.Expect(204, "DELETE", fmt.Sprintf("%s/%d", exerciseSessionsPath(w, ws), es.ID), nil)

	if sets, _ := env.Store.Sets(context.Background(), es.ID); len(sets) != 0 {
		t.Errorf("sets survived exercise-session delete: %+v", sets)
	}
}

func TestBulkCreateExerciseSessionsIsIdempotent(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")
	a := createExercise(t, c, w, "A")
	b := createExercise(t, c, w, "B")
	ws := createSession(t, c, w)

	existing := createExerciseSession(t, c, w, ws, a)

	var list []model.ExerciseSession
	c.Expect(201, "POST", exerciseSessionsPath(w, ws)+"/bulk", map[string][]int{"exerciseIds": {a.ID, b.ID, b.ID}}).JSON(t, &list)

	if len(list) != 2 || list[0].ID != existing.ID || list[1].ExerciseID != b.ID || list[1].Sets == nil {
		t.Fatalf("bulk = %+v", list)
	}

	// Calling again (a double fetch in the app) creates nothing new.
	c.Expect(201, "POST", exerciseSessionsPath(w, ws)+"/bulk", map[string][]int{"exerciseIds": {a.ID, b.ID}}).JSON(t, &list)

	if len(list) != 2 {
		t.Errorf("second bulk created duplicates: %+v", list)
	}
}

func TestSets(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")
	ex := createExercise(t, c, w, "Bench", "isWeight", "isReps")
	ws := createSession(t, c, w)
	es := createExerciseSession(t, c, w, ws, ex)
	path := setsPath(w, ws, es)

	// Tracked metrics seed to 0 -- present, not omitted -- and the rest are
	// explicit nulls. The app hides any input whose value is null.
	res := c.Expect(201, "POST", path, nil)
	for _, want := range []string{`"weight":0`, `"reps":0`, `"duration":null`, `"distance":null`} {
		if !strings.Contains(string(res.Body), want) {
			t.Errorf("new set %s missing %s", res.Body, want)
		}
	}

	var first model.SetSession
	res.JSON(t, &first)

	var got model.SetSession
	c.Expect(200, "PATCH", fmt.Sprintf("%s/%d/weight", path, first.ID), map[string]float64{"weight": 102.5}).JSON(t, &got)

	if got.Weight == nil || *got.Weight != 102.5 || got.Reps == nil || *got.Reps != 0 {
		t.Errorf("after weight patch = %+v", got)
	}

	for field, body := range map[string]any{
		"weight": map[string]any{"weight": nil}, // clearing must not persist null
		"reps":   map[string]any{"reps": 2.5},
		"order":  map[string]any{"order": 1},
	} {
		if res := c.Do("PATCH", fmt.Sprintf("%s/%d/%s", path, first.ID, field), body); res.Status != 400 {
			t.Errorf("patch %v = %s", body, res)
		}
	}

	second := addSet(t, c, w, ws, es, map[string]any{"reps": 8})
	third := addSet(t, c, w, ws, es, nil)

	var list []model.SetSession
	c.Expect(200, "GET", path, nil).JSON(t, &list)

	// Creation order: the app treats the last element as the previous set.
	if len(list) != 3 || list[0].ID != first.ID || list[1].ID != second.ID || list[2].ID != third.ID {
		t.Errorf("list order = %+v", list)
	}

	c.Expect(204, "DELETE", fmt.Sprintf("%s/%d", path, second.ID), nil)

	if res := c.Do("DELETE", fmt.Sprintf("%s/%d", path, second.ID), nil); res.Code() != "SETSESSION_NOT_FOUND" {
		t.Errorf("second delete = %s", res)
	}

	if res := c.Do("GET", fmt.Sprintf("%s/%d", path, second.ID), nil); res.Code() != "SETSESSION_NOT_FOUND" {
		t.Errorf("get deleted = %s", res)
	}

	intruder, _ := env.User()
	if res := intruder.Do("PATCH", fmt.Sprintf("%s/%d/weight", path, first.ID), map[string]int{"weight": 1}); res.Code() != "EXERCISESESSION_NOT_FOUND" {
		t.Errorf("another user's set = %s", res)
	}

	// Flags are read live: enabling a metric mid-workout seeds the next set.
	c.Expect(200, "PATCH", fmt.Sprintf("%s/%d/isDistance", exercisesPath(w), ex.ID), map[string]bool{"isDistance": true})

	if res := c.Expect(201, "POST", path, nil); !strings.Contains(string(res.Body), `"distance":0`) {
		t.Errorf("set after enabling distance = %s", res.Body)
	}

	assertAggregatesMatchSets(t, env, ws.ID, es.ID)
}

// The aggregates are maintained incrementally; they must always equal a
// recompute from the sets.
func TestAggregatesStayConsistent(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Run")
	ex := createExercise(t, c, w, "Intervals", "isDistance", "isDuration")
	ws := createSession(t, c, w)
	es := createExerciseSession(t, c, w, ws, ex)
	path := setsPath(w, ws, es)

	a := addSet(t, c, w, ws, es, map[string]any{"distance": 1.5, "duration": 300})
	b := addSet(t, c, w, ws, es, map[string]any{"distance": 2.25})
	addSet(t, c, w, ws, es, nil)

	c.Expect(200, "PATCH", fmt.Sprintf("%s/%d", path, a.ID), map[string]any{"distance": 3, "duration": 280})
	c.Expect(200, "PATCH", fmt.Sprintf("%s/%d/distance", path, b.ID), map[string]float64{"distance": 2.25}) // no-op
	c.Expect(204, "DELETE", fmt.Sprintf("%s/%d", path, b.ID), nil)

	assertAggregatesMatchSets(t, env, ws.ID, es.ID)
}

func TestAverages(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")
	ex := createExercise(t, c, w, "Bench", "isWeight")

	logAt := func(date string, weights ...float64) model.WorkoutSession {
		ws := createSessionAt(t, c, w, date)
		es := createExerciseSession(t, c, w, ws, ex)

		for _, kg := range weights {
			addSet(t, c, w, ws, es, map[string]any{"weight": kg})
		}

		return ws
	}

	logAt("2031-06-13T10:00:00.000Z", 10)
	logAt("2031-06-12T10:00:00.000Z", 20, 40)
	logAt("2031-06-14T10:00:00.000Z")      // no sets: drops out entirely
	logAt("2031-05-01T10:00:00.000Z", 100) // outside the 7-day window
	current := logAt("2031-06-15T11:00:00.000Z", 1000)

	path := fmt.Sprintf("%s/%d/averages?excludeSessionId=%d", exercisesPath(w), ex.ID, current.ID)

	// (10 + mean(20, 40)) / 2 = 20 -- a sum of per-session means, not 70/3.
	if res := c.Expect(200, "GET", path, nil); string(res.Body) != `{"weight":20,"reps":null,"duration":null,"distance":null}` {
		t.Errorf("weekly = %s", res.Body)
	}

	var all model.Averages
	c.Expect(200, "GET", strings.Replace(path, "/averages?", "/averages/all?", 1), nil).JSON(t, &all)

	if all.Weight == nil || math.Abs(*all.Weight-(10+30+100)/3.0) > 1e-9 {
		t.Errorf("all-time weight = %v", all.Weight)
	}

	for _, bad := range []string{"", "?excludeSessionId=0", "?excludeSessionId=x"} {
		p := fmt.Sprintf("%s/%d/averages%s", exercisesPath(w), ex.ID, bad)
		if res := c.Do("GET", p, nil); res.Code() != "VALIDATION_ERROR" {
			t.Errorf("%s = %s", p, res)
		}
	}
}

func TestPreviousSetCount(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")
	ex := createExercise(t, c, w, "Bench")

	count := func(ws model.WorkoutSession, es model.ExerciseSession) int {
		var res model.CountResponse
		c.Expect(200, "GET", fmt.Sprintf("%s/%d/previous-set-count", exerciseSessionsPath(w, ws), es.ID), nil).JSON(t, &res)

		return res.Count
	}

	ws1 := createSessionAt(t, c, w, "2031-06-01T10:00:00.000Z")
	es1 := createExerciseSession(t, c, w, ws1, ex)

	// Nothing before it: 1, not 0, as the original defaulted.
	if got := count(ws1, es1); got != 1 {
		t.Errorf("first ever = %d, want 1", got)
	}

	addSet(t, c, w, ws1, es1, nil)
	addSet(t, c, w, ws1, es1, nil)

	ws2 := createSessionAt(t, c, w, "2031-06-08T10:00:00.000Z")
	es2 := createExerciseSession(t, c, w, ws2, ex)

	if got := count(ws2, es2); got != 2 {
		t.Errorf("after a 2-set session = %d, want 2", got)
	}

	// A preceding session with no sets counts as 0.
	ws3 := createSessionAt(t, c, w, "2031-06-10T10:00:00.000Z")
	es3 := createExerciseSession(t, c, w, ws3, ex)

	if got := count(ws3, es3); got != 0 {
		t.Errorf("after an empty session = %d, want 0", got)
	}
}

// Rows written by the C# service carry no aggregates. They are backfilled from
// their sets on first touch, and set writes never apply a delta to a row that
// has not been backfilled.
func TestLegacyRowsAreBackfilled(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")
	ex := createExercise(t, c, w, "Bench", "isWeight")
	old := createSessionAt(t, c, w, "2031-06-10T10:00:00.000Z")
	cur := createSessionAt(t, c, w, "2031-06-15T10:00:00.000Z")

	legacy := putLegacyExerciseSession(t, env, old, ex, 10, 30)

	path := fmt.Sprintf("%s/%d/averages/all?excludeSessionId=%d", exercisesPath(w), ex.ID, cur.ID)
	if res := c.Expect(200, "GET", path, nil); !strings.HasPrefix(string(res.Body), `{"weight":20,`) {
		t.Errorf("averages over a legacy row = %s", res.Body)
	}

	row, _ := env.Store.ExerciseSession(context.Background(), old.ID, legacy.ID)
	if row == nil || row.SetCount == nil || *row.SetCount != 2 {
		t.Errorf("legacy row not backfilled: %+v", row)
	}

	es := createExerciseSession(t, c, w, cur, ex)

	var n model.CountResponse
	c.Expect(200, "GET", fmt.Sprintf("%s/%d/previous-set-count", exerciseSessionsPath(w, cur), es.ID), nil).JSON(t, &n)

	if n.Count != 2 {
		t.Errorf("previous-set-count over a legacy row = %d", n.Count)
	}

	// A set write on a not-yet-backfilled row backfills first.
	other := putLegacyExerciseSession(t, env, old, ex, 50)
	addSet(t, c, w, old, other, map[string]any{"weight": 70})

	assertAggregatesMatchSets(t, env, old.ID, other.ID)
}

func TestHistory(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")
	ex := createExercise(t, c, w, "Bench", "isWeight")

	june := createSessionAt(t, c, w, "2031-06-10T10:00:00.000Z")
	es := createExerciseSession(t, c, w, june, ex)
	addSet(t, c, w, june, es, map[string]any{"weight": 60})

	edge := createSessionAt(t, c, w, "2031-06-30T23:59:59.999Z") // last instant of June, UTC
	createSessionAt(t, c, w, "2031-07-01T00:00:00.000Z")         // first of July: excluded
	createSessionAt(t, c, w, "2031-05-31T23:59:59.999Z")         // May: excluded

	var sessions []model.HistorySession
	res := c.Expect(200, "GET", "/history/sessions?month=2031-06", nil)
	res.JSON(t, &sessions)

	if len(sessions) != 2 || sessions[0].ID != june.ID || sessions[1].ID != edge.ID {
		t.Fatalf("june = %s", res.Body)
	}

	// The History screen builds paths from the top-level workoutId.
	if !strings.Contains(string(res.Body), fmt.Sprintf(`"workoutId":%d`, w.ID)) {
		t.Errorf("history item missing workoutId: %s", res.Body)
	}

	got := sessions[0]
	if got.Workout.Name != "Push" || len(got.Exercises) != 1 || got.Exercises[0].Exercise.Name != "Bench" ||
		len(got.Exercises[0].Sets) != 1 || *got.Exercises[0].Sets[0].Weight != 60 {
		t.Errorf("june session = %+v", got)
	}

	if !strings.Contains(string(res.Body), `"exercises":[]`) {
		t.Errorf("session with no exercises should carry an empty array: %s", res.Body)
	}

	// Renames show through, as names are read live.
	c.Expect(200, "PATCH", fmt.Sprintf("/workouts/%d/name", w.ID), map[string]string{"name": "Push Day"})
	c.Expect(200, "GET", "/history/sessions?month=2031-06", nil).JSON(t, &sessions)

	if sessions[0].Workout.Name != "Push Day" {
		t.Errorf("rename not reflected: %q", sessions[0].Workout.Name)
	}

	if res := c.Expect(200, "GET", "/history/sessions?month=2025-01", nil); string(res.Body) != "[]" {
		t.Errorf("empty month = %s", res.Body)
	}

	for _, bad := range []string{"", "?month=2031-13", "?month=2031-6", "?month=june"} {
		if res := c.Do("GET", "/history/sessions"+bad, nil); res.Code() != "VALIDATION_ERROR" {
			t.Errorf("month %q = %s", bad, res)
		}
	}
}

func TestDeleteSessionCascades(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	w := createWorkout(t, c, "Push")
	ex := createExercise(t, c, w, "Bench")
	ws := createSession(t, c, w)
	es := createExerciseSession(t, c, w, ws, ex)
	addSet(t, c, w, ws, es, nil)

	c.Expect(204, "DELETE", fmt.Sprintf("%s/%d", sessionsPath(w), ws.ID), nil)

	if res := c.Do("DELETE", fmt.Sprintf("%s/%d", sessionsPath(w), ws.ID), nil); res.Code() != "WORKOUTSESSION_NOT_FOUND" {
		t.Errorf("second delete = %s", res)
	}

	if row, _ := env.Store.ExerciseSession(context.Background(), ws.ID, es.ID); row != nil {
		t.Error("exercise session survived")
	}

	if sets, _ := env.Store.Sets(context.Background(), es.ID); len(sets) != 0 {
		t.Error("sets survived")
	}
}

func assertAggregatesMatchSets(t *testing.T, env *testutil.Env, workoutSessionID, id int) {
	t.Helper()

	ctx := context.Background()

	row, err := env.Store.ExerciseSession(ctx, workoutSessionID, id)
	if err != nil || row == nil {
		t.Fatalf("exercise session: %+v %v", row, err)
	}

	sets, err := env.Store.Sets(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	want := model.AggregatesOf(sets)

	if row.SetCount == nil || *row.SetCount != *want.SetCount ||
		math.Abs(row.WeightSum-want.WeightSum) > 1e-9 || row.WeightCount != want.WeightCount ||
		math.Abs(row.RepsSum-want.RepsSum) > 1e-9 || row.RepsCount != want.RepsCount ||
		math.Abs(row.DurationSum-want.DurationSum) > 1e-9 || row.DurationCount != want.DurationCount ||
		math.Abs(row.DistanceSum-want.DistanceSum) > 1e-9 || row.DistanceCount != want.DistanceCount {
		t.Errorf("stored aggregates %+v != recomputed %+v", row.Aggregates, want)
	}
}

// legacyIDs hands out ids far above anything the counters reach in a test run.
var legacyIDs atomic.Int64

func init() { legacyIDs.Store(900_000_000) }

// putLegacyExerciseSession writes an exercise session and its sets the way the
// C# service did: no aggregate attributes at all.
func putLegacyExerciseSession(t *testing.T, env *testutil.Env, ws model.WorkoutSession, ex model.Exercise, weights ...float64) model.ExerciseSession {
	t.Helper()

	ctx := context.Background()

	base := int(legacyIDs.Add(1000))
	es := model.ExerciseSession{
		ID: base, WorkoutSessionID: ws.ID, ExerciseID: ex.ID, CreatedAt: jsonx.New(env.Clock.Now()),
		UserID: ws.UserID, WorkoutID: ws.WorkoutID, SessionDate: ws.Date,
	}

	item, err := attributevalue.MarshalMap(es)
	if err != nil {
		t.Fatal(err)
	}

	for _, attr := range []string{"setCount", "weightSum", "weightCount", "repsSum", "repsCount", "durationSum", "durationCount", "distanceSum", "distanceCount"} {
		delete(item, attr)
	}

	if _, err := env.DB.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(env.DB.T.ExerciseSessions), Item: item}); err != nil {
		t.Fatal(err)
	}

	for i, kg := range weights {
		set := model.SetSession{ID: base + 1 + i, Weight: &kg, ExerciseSessionID: es.ID, CreatedAt: es.CreatedAt}

		setItem, err := attributevalue.MarshalMap(set)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := env.DB.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(env.DB.T.SetSessions), Item: setItem}); err != nil {
			t.Fatal(err)
		}
	}

	return es
}
