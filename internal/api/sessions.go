package api

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/model"
	"github.com/dtmesa/gravappbe/internal/valid"
)

// Workout sessions. As in the original, the reads scope by session id and
// user, and do not require the session to belong to the workout in the URL --
// only POST checks it.

func (s *Server) createSession(w http.ResponseWriter, r *http.Request, u model.User) error {
	workoutID, err := httpx.PathID(r, "workoutId")
	if err != nil {
		return err
	}

	// The body is optional: {} or nothing means "now".
	var req valid.CreateWorkoutSession

	body, err := httpx.ReadBody(w, r)
	if err != nil {
		return err
	}

	if body != nil {
		if err := httpx.Decode(body, &req); err != nil {
			return err
		}
	}

	workout, err := s.Store.RequireWorkout(r.Context(), u.ID, workoutID)
	if err != nil {
		return err
	}

	ws, err := s.Store.CreateWorkoutSession(r.Context(), u.ID, workoutID, req.Date)
	if err != nil {
		return err
	}

	ws.Workout = &workout

	httpx.WriteJSON(w, http.StatusCreated, ws)

	return nil
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request, u model.User) error {
	id, err := sessionPath(r, "id")
	if err != nil {
		return err
	}

	ws, err := s.Store.RequireWorkoutSession(r.Context(), u.ID, id)
	if err != nil {
		return err
	}

	// May be nil if the workout is gone, in which case it is omitted.
	if ws.Workout, err = s.Store.Workout(r.Context(), u.ID, ws.WorkoutID); err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, ws)

	return nil
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request, u model.User) error {
	id, err := sessionPath(r, "id")
	if err != nil {
		return err
	}

	if _, err := s.Store.RequireWorkoutSession(r.Context(), u.ID, id); err != nil {
		return err
	}

	if err := s.Store.DeleteWorkoutSession(r.Context(), u.ID, id); err != nil {
		return err
	}

	httpx.NoContent(w)

	return nil
}

func (s *Server) patchSessionField(w http.ResponseWriter, r *http.Request, u model.User) error {
	id, err := sessionPath(r, "id")
	if err != nil {
		return err
	}

	body, err := httpx.ReadBody(w, r)
	if err != nil {
		return err
	}

	date, err := valid.SessionDate(r.PathValue("field"), body)
	if err != nil {
		return err
	}

	ws, err := s.Store.UpdateSessionDate(r.Context(), u.ID, id, date)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, ws)

	return nil
}

// sessionPath reads a session-route id. The {workoutId} segment is not used on
// these routes but must still be an integer, as the {workoutId:int} route
// constraint required.
func sessionPath(r *http.Request, name string) (int, error) {
	if _, err := strconv.Atoi(r.PathValue("workoutId")); err != nil {
		return 0, httpx.New(http.StatusNotFound, "NOT_FOUND", "Route not found")
	}

	return httpx.PathID(r, name)
}

// Exercise sessions.

func (s *Server) createExerciseSession(w http.ResponseWriter, r *http.Request, u model.User) error {
	sessionID, workoutID, err := createPath(r)
	if err != nil {
		return err
	}

	req, err := decode[valid.CreateExerciseSession](w, r)
	if err != nil {
		return err
	}

	ws, err := s.sessionInWorkout(r, u, sessionID, workoutID)
	if err != nil {
		return err
	}

	if _, err := s.Store.RequireExercise(r.Context(), workoutID, req.ExerciseID); err != nil {
		return err
	}

	created, err := s.Store.CreateExerciseSessions(r.Context(), *ws, []int{req.ExerciseID})
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusCreated, created[0])

	return nil
}

// bulkCreateExerciseSessions (additive) creates exercise sessions for every
// listed exercise not already in the session, in one transaction, and
// returns the session's full list. It replaces the N parallel creates the app
// fires when starting a workout, and the duplicates those could produce.
func (s *Server) bulkCreateExerciseSessions(w http.ResponseWriter, r *http.Request, u model.User) error {
	sessionID, workoutID, err := createPath(r)
	if err != nil {
		return err
	}

	req, err := decode[valid.BulkExerciseSessions](w, r)
	if err != nil {
		return err
	}

	ws, err := s.sessionInWorkout(r, u, sessionID, workoutID)
	if err != nil {
		return err
	}

	g, gctx := errgroup.WithContext(r.Context())
	g.SetLimit(16)

	for _, exerciseID := range req.ExerciseIDs {
		g.Go(func() error {
			_, err := s.Store.RequireExercise(gctx, workoutID, exerciseID)
			return err
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	existing, err := s.Store.ExerciseSessions(r.Context(), ws.ID)
	if err != nil {
		return err
	}

	present := map[int]bool{}
	for _, es := range existing {
		present[es.ExerciseID] = true
	}

	var missing []int

	for _, id := range req.ExerciseIDs {
		if !present[id] {
			present[id] = true
			missing = append(missing, id)
		}
	}

	if len(missing) > 0 {
		created, err := s.Store.CreateExerciseSessions(r.Context(), *ws, missing)
		if err != nil {
			return err
		}

		existing = append(existing, created...)
	}

	if err := s.withSets(r, existing); err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusCreated, existing)

	return nil
}

// createPath reads the session and workout ids an exercise-session create
// targets.
func createPath(r *http.Request) (sessionID, workoutID int, err error) {
	if sessionID, err = httpx.PathID(r, "sessionId"); err != nil {
		return 0, 0, err
	}

	if workoutID, err = strconv.Atoi(r.PathValue("workoutId")); err != nil {
		return 0, 0, httpx.New(http.StatusNotFound, "NOT_FOUND", "Route not found")
	}

	return sessionID, workoutID, nil
}

// sessionInWorkout loads the session and requires it to belong to the workout
// in the URL. The original checked that the exercise belonged to that workout
// and the session to the user, but never the session to the workout -- so
// workout A's exercise could be logged into workout B's session. The app
// always sends a matching pair.
func (s *Server) sessionInWorkout(r *http.Request, u model.User, sessionID, workoutID int) (*model.WorkoutSession, error) {
	ws, err := s.Store.RequireWorkoutSession(r.Context(), u.ID, sessionID)
	if err != nil {
		return nil, err
	}

	if ws.WorkoutID != workoutID {
		return nil, httpx.NotFound("Workout session", "WORKOUTSESSION_NOT_FOUND")
	}

	return &ws, nil
}

func (s *Server) listExerciseSessions(w http.ResponseWriter, r *http.Request, u model.User) error {
	sessionID, err := sessionPath(r, "sessionId")
	if err != nil {
		return err
	}

	if _, err := s.Store.RequireWorkoutSession(r.Context(), u.ID, sessionID); err != nil {
		return err
	}

	list, err := s.Store.ExerciseSessions(r.Context(), sessionID)
	if err != nil {
		return err
	}

	if err := s.withSets(r, list); err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, list)

	return nil
}

func (s *Server) getExerciseSession(w http.ResponseWriter, r *http.Request, u model.User) error {
	es, err := s.ownedExerciseSession(r, u, "id")
	if err != nil {
		return err
	}

	list := []model.ExerciseSession{es}
	if err := s.withSets(r, list); err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, list[0])

	return nil
}

func (s *Server) deleteExerciseSession(w http.ResponseWriter, r *http.Request, u model.User) error {
	es, err := s.ownedExerciseSession(r, u, "id")
	if err != nil {
		return err
	}

	if err := s.Store.DeleteExerciseSession(r.Context(), es.WorkoutSessionID, es.ID); err != nil {
		return err
	}

	httpx.NoContent(w)

	return nil
}

// previousSetCount is how many sets were logged the last time this exercise
// was done in this workout. It answers from the aggregate set count, with no
// set query. When nothing precedes this session the answer is 1, not 0, as
// the original defaulted.
func (s *Server) previousSetCount(w http.ResponseWriter, r *http.Request, u model.User) error {
	cur, err := s.ownedExerciseSession(r, u, "id")
	if err != nil {
		return err
	}

	prev, err := s.Store.PreviousExerciseSession(r.Context(), cur.ExerciseID, cur.WorkoutID, u.ID, cur.ID)
	if err != nil {
		return err
	}

	count := 1

	if prev != nil {
		if err := s.Store.EnsureAggregates(r.Context(), prev); err != nil {
			return err
		}

		count = prev.Count()
	}

	httpx.WriteJSON(w, http.StatusOK, model.CountResponse{Count: count})

	return nil
}

// ownedExerciseSession resolves an exercise session and proves the caller owns
// it. Exercise sessions carry their owner's userId, so this is one read where
// the original read the parent session first.
func (s *Server) ownedExerciseSession(r *http.Request, u model.User, idParam string) (model.ExerciseSession, error) {
	sessionID, err := sessionPath(r, "sessionId")
	if err != nil {
		return model.ExerciseSession{}, err
	}

	id, err := httpx.PathID(r, idParam)
	if err != nil {
		return model.ExerciseSession{}, err
	}

	es, err := s.Store.ExerciseSession(r.Context(), sessionID, id)
	if err != nil {
		return model.ExerciseSession{}, err
	}

	if es == nil || es.UserID != u.ID {
		return model.ExerciseSession{}, httpx.NotFound("Exercise session", "EXERCISESESSION_NOT_FOUND")
	}

	return *es, nil
}

// withSets fills each exercise session's sets, in parallel. Sets is always a
// non-nil slice: the client calls .map() on it unguarded.
func (s *Server) withSets(r *http.Request, list []model.ExerciseSession) error {
	g, ctx := errgroup.WithContext(r.Context())
	g.SetLimit(16)

	for i := range list {
		g.Go(func() error {
			sets, err := s.Store.Sets(ctx, list[i].ID)
			list[i].Sets = sets

			return err
		})
	}

	return g.Wait()
}

// History.

var monthPattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// history returns the sessions in a calendar month. Boundaries are explicit
// UTC: the original used the server's local zone, which on Lambda is UTC, so
// this matches production while no longer shifting on a developer machine.
func (s *Server) history(w http.ResponseWriter, r *http.Request, u model.User) error {
	month := r.URL.Query().Get("month")
	if !monthPattern.MatchString(month) {
		return httpx.Invalid("month", "month must be formatted YYYY-MM")
	}

	start, err := time.ParseInLocation("2006-01", month, time.UTC)
	if err != nil {
		return httpx.Invalid("month", "month must be formatted YYYY-MM")
	}

	sessions, err := s.Store.History(r.Context(), u.ID, start, start.AddDate(0, 1, 0))
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, sessions)

	return nil
}
