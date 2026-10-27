package api

import (
	"net/http"
	"strconv"

	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/model"
	"github.com/dtmesa/gravappbe/internal/valid"
)

func (s *Server) createWorkout(w http.ResponseWriter, r *http.Request, u model.User) error {
	req, err := decode[valid.CreateNamed](w, r)
	if err != nil {
		return err
	}

	workout, err := s.Store.CreateWorkout(r.Context(), u.ID, req.Name)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusCreated, workout)

	return nil
}

func (s *Server) listWorkouts(w http.ResponseWriter, r *http.Request, u model.User) error {
	workouts, err := s.Store.Workouts(r.Context(), u.ID)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, workouts)

	return nil
}

func (s *Server) getWorkout(w http.ResponseWriter, r *http.Request, u model.User) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}

	workout, err := s.Store.RequireWorkout(r.Context(), u.ID, id)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, workout)

	return nil
}

func (s *Server) deleteWorkout(w http.ResponseWriter, r *http.Request, u model.User) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}

	workout, err := s.Store.RequireWorkout(r.Context(), u.ID, id)
	if err != nil {
		return err
	}

	if err := s.Store.DeleteWorkout(r.Context(), workout); err != nil {
		return err
	}

	httpx.NoContent(w)

	return nil
}

// patchWorkoutField is the live PATCH /workouts/{id}/{field} contract.
func (s *Server) patchWorkoutField(w http.ResponseWriter, r *http.Request, u model.User) error {
	return s.applyWorkoutPatch(w, r, u, func(body []byte) (valid.WorkoutPatch, error) {
		return valid.WorkoutField(r.PathValue("field"), body)
	})
}

// patchWorkout is the sparse-body equivalent (additive).
func (s *Server) patchWorkout(w http.ResponseWriter, r *http.Request, u model.User) error {
	return s.applyWorkoutPatch(w, r, u, valid.WorkoutSparse)
}

func (s *Server) applyWorkoutPatch(w http.ResponseWriter, r *http.Request, u model.User, parse func([]byte) (valid.WorkoutPatch, error)) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}

	body, err := httpx.ReadBody(w, r)
	if err != nil {
		return err
	}

	patch, err := parse(body)
	if err != nil {
		return err
	}

	workout, err := s.Store.UpdateWorkout(r.Context(), u.ID, id, patch)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, workout)

	return nil
}

func (s *Server) reorderWorkouts(w http.ResponseWriter, r *http.Request, u model.User) error {
	req, err := decode[valid.Reorder](w, r)
	if err != nil {
		return err
	}

	if err := s.Store.ReorderWorkouts(r.Context(), u.ID, req.IDs); err != nil {
		return err
	}

	httpx.NoContent(w)

	return nil
}

// Exercises. Every route first proves the caller owns the workout in the URL;
// exercises are then resolved within that workout.

func (s *Server) ownedWorkout(r *http.Request, u model.User) (model.Workout, error) {
	workoutID, err := httpx.PathID(r, "workoutId")
	if err != nil {
		return model.Workout{}, err
	}

	return s.Store.RequireWorkout(r.Context(), u.ID, workoutID)
}

func (s *Server) createExercise(w http.ResponseWriter, r *http.Request, u model.User) error {
	workoutID, err := httpx.PathID(r, "workoutId")
	if err != nil {
		return err
	}

	req, err := decode[valid.CreateNamed](w, r)
	if err != nil {
		return err
	}

	if _, err := s.Store.RequireWorkout(r.Context(), u.ID, workoutID); err != nil {
		return err
	}

	ex, err := s.Store.CreateExercise(r.Context(), workoutID, req.Name)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusCreated, ex)

	return nil
}

func (s *Server) listExercises(w http.ResponseWriter, r *http.Request, u model.User) error {
	workout, err := s.ownedWorkout(r, u)
	if err != nil {
		return err
	}

	exercises, err := s.Store.Exercises(r.Context(), workout.ID)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, exercises)

	return nil
}

func (s *Server) getExercise(w http.ResponseWriter, r *http.Request, u model.User) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}

	workout, err := s.ownedWorkout(r, u)
	if err != nil {
		return err
	}

	ex, err := s.Store.RequireExercise(r.Context(), workout.ID, id)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, ex)

	return nil
}

func (s *Server) deleteExercise(w http.ResponseWriter, r *http.Request, u model.User) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}

	workout, err := s.ownedWorkout(r, u)
	if err != nil {
		return err
	}

	ex, err := s.Store.RequireExercise(r.Context(), workout.ID, id)
	if err != nil {
		return err
	}

	if err := s.Store.DeleteExercise(r.Context(), ex); err != nil {
		return err
	}

	httpx.NoContent(w)

	return nil
}

func (s *Server) patchExerciseField(w http.ResponseWriter, r *http.Request, u model.User) error {
	return s.applyExercisePatch(w, r, u, func(body []byte) (valid.ExercisePatch, error) {
		return valid.ExerciseField(r.PathValue("field"), body)
	})
}

func (s *Server) patchExercise(w http.ResponseWriter, r *http.Request, u model.User) error {
	return s.applyExercisePatch(w, r, u, valid.ExerciseSparse)
}

// applyExercisePatch validates the body before checking ownership, matching
// the original order (a bad body is a 400 even on someone else's workout).
func (s *Server) applyExercisePatch(w http.ResponseWriter, r *http.Request, u model.User, parse func([]byte) (valid.ExercisePatch, error)) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}

	body, err := httpx.ReadBody(w, r)
	if err != nil {
		return err
	}

	patch, err := parse(body)
	if err != nil {
		return err
	}

	workout, err := s.ownedWorkout(r, u)
	if err != nil {
		return err
	}

	ex, err := s.Store.UpdateExercise(r.Context(), workout.ID, id, patch)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, ex)

	return nil
}

func (s *Server) reorderExercises(w http.ResponseWriter, r *http.Request, u model.User) error {
	req, err := decode[valid.Reorder](w, r)
	if err != nil {
		return err
	}

	workout, err := s.ownedWorkout(r, u)
	if err != nil {
		return err
	}

	if err := s.Store.ReorderExercises(r.Context(), workout.ID, req.IDs); err != nil {
		return err
	}

	httpx.NoContent(w)

	return nil
}

// averages serves /averages (the last seven days) and /averages/all. Both
// read the per-exercise-session aggregates, so the cost is one index query
// rather than one set query per session the user has ever logged.
func (s *Server) averages(allTime bool) authedFunc {
	return func(w http.ResponseWriter, r *http.Request, u model.User) error {
		id, err := httpx.PathID(r, "id")
		if err != nil {
			return err
		}

		// The original schema made this required, not optional.
		exclude, err := strconv.Atoi(r.URL.Query().Get("excludeSessionId"))
		if err != nil || exclude <= 0 {
			return httpx.Invalid("excludeSessionId", "excludeSessionId must be a positive integer")
		}

		workout, err := s.ownedWorkout(r, u)
		if err != nil {
			return err
		}

		ex, err := s.Store.RequireExercise(r.Context(), workout.ID, id)
		if err != nil {
			return err
		}

		since := s.Now().AddDate(0, 0, -7)
		window := &since

		if allTime {
			window = nil
		}

		sessions, err := s.Store.ExerciseSessionsForExercise(r.Context(), ex.ID, u.ID, window, exclude)
		if err != nil {
			return err
		}

		if err := s.Store.EnsureAllAggregates(r.Context(), sessions); err != nil {
			return err
		}

		aggs := make([]model.Aggregates, len(sessions))
		for i, es := range sessions {
			aggs[i] = es.Aggregates
		}

		httpx.WriteJSON(w, http.StatusOK, model.ComputeAverages(aggs, ex))

		return nil
	}
}
