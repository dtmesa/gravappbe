package api

import (
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/model"
	"github.com/dtmesa/gravappbe/internal/valid"
)

// Set sessions -- the most-executed path in the app, written continuously
// during a workout. Ownership is proven by the parent exercise session's
// userId, one read where the original chained three (session, exercise
// session, set). Writes keep the parent's aggregates current in the same
// transaction.

func (s *Server) createSet(w http.ResponseWriter, r *http.Request, u model.User) error {
	es, err := s.ownedExerciseSession(r, u, "exerciseSessionId")
	if err != nil {
		return err
	}

	// Which metrics a new set seeds with 0 depends on the exercise's current
	// flags -- read live, so changing them mid-workout affects the next set.
	var ex model.Exercise

	g, ctx := errgroup.WithContext(r.Context())
	g.Go(func() error { return s.Store.EnsureAggregates(ctx, &es) })
	g.Go(func() (err error) {
		ex, err = s.Store.RequireExercise(ctx, es.WorkoutID, es.ExerciseID)
		return err
	})

	if err := g.Wait(); err != nil {
		return err
	}

	set, err := s.Store.CreateSet(r.Context(), es, ex)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusCreated, set)

	return nil
}

func (s *Server) listSets(w http.ResponseWriter, r *http.Request, u model.User) error {
	es, err := s.ownedExerciseSession(r, u, "exerciseSessionId")
	if err != nil {
		return err
	}

	sets, err := s.Store.Sets(r.Context(), es.ID)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, sets)

	return nil
}

func (s *Server) getSet(w http.ResponseWriter, r *http.Request, u model.User) error {
	_, set, err := s.ownedSet(r, u)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, set)

	return nil
}

func (s *Server) deleteSet(w http.ResponseWriter, r *http.Request, u model.User) error {
	es, set, err := s.ownedSet(r, u)
	if err != nil {
		return err
	}

	if err := s.Store.EnsureAggregates(r.Context(), &es); err != nil {
		return err
	}

	if err := s.Store.DeleteSet(r.Context(), es, set); err != nil {
		return err
	}

	httpx.NoContent(w)

	return nil
}

func (s *Server) patchSetField(w http.ResponseWriter, r *http.Request, u model.User) error {
	return s.applySetPatch(w, r, u, func(body []byte) (valid.SetPatch, error) {
		return valid.SetField(r.PathValue("field"), body)
	})
}

func (s *Server) patchSet(w http.ResponseWriter, r *http.Request, u model.User) error {
	return s.applySetPatch(w, r, u, valid.SetSparse)
}

// applySetPatch validates before checking ownership, as the original did.
func (s *Server) applySetPatch(w http.ResponseWriter, r *http.Request, u model.User, parse func([]byte) (valid.SetPatch, error)) error {
	if _, err := httpx.PathID(r, "id"); err != nil {
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

	es, set, err := s.ownedSet(r, u)
	if err != nil {
		return err
	}

	if err := s.Store.EnsureAggregates(r.Context(), &es); err != nil {
		return err
	}

	updated, err := s.Store.UpdateSet(r.Context(), es, set, patch)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, updated)

	return nil
}

// ownedSet reads the parent exercise session and the set in parallel, then
// checks ownership -- one round trip instead of the original's three.
func (s *Server) ownedSet(r *http.Request, u model.User) (model.ExerciseSession, model.SetSession, error) {
	sessionID, err := sessionPath(r, "sessionId")
	if err != nil {
		return model.ExerciseSession{}, model.SetSession{}, err
	}

	esID, err := httpx.PathID(r, "exerciseSessionId")
	if err != nil {
		return model.ExerciseSession{}, model.SetSession{}, err
	}

	id, err := httpx.PathID(r, "id")
	if err != nil {
		return model.ExerciseSession{}, model.SetSession{}, err
	}

	var (
		es  *model.ExerciseSession
		set *model.SetSession
	)

	g, ctx := errgroup.WithContext(r.Context())
	g.Go(func() (err error) { es, err = s.Store.ExerciseSession(ctx, sessionID, esID); return err })
	g.Go(func() (err error) { set, err = s.Store.Set(ctx, esID, id); return err })

	if err := g.Wait(); err != nil {
		return model.ExerciseSession{}, model.SetSession{}, err
	}

	if es == nil || es.UserID != u.ID {
		return model.ExerciseSession{}, model.SetSession{}, httpx.NotFound("Exercise session", "EXERCISESESSION_NOT_FOUND")
	}

	if set == nil {
		return model.ExerciseSession{}, model.SetSession{}, httpx.NotFound("Set session", "SETSESSION_NOT_FOUND")
	}

	return *es, *set, nil
}
