package api

import (
	"net/http"

	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/model"
)

// routes registers every endpoint. The legacy shapes the current app calls are
// kept exactly; routes marked "additive" are new and unused by today's app.
func (s *Server) routes(mux *http.ServeMux) {
	rl := &s.limits

	mux.Handle("GET /{$}", httpx.Handle(func(w http.ResponseWriter, _ *http.Request) error {
		httpx.WriteJSON(w, http.StatusOK, model.StatusResponse{Status: "ok"})
		return nil
	}))

	// Account
	mux.Handle("POST /auth/register", s.public(&rl.Register, s.register))
	mux.Handle("POST /auth/login", s.public(&rl.Login, s.login))
	mux.Handle("GET /auth/me", s.private(nil, s.me))
	mux.Handle("PATCH /auth/username", s.private(&rl.AuthMutate, s.updateUsername))
	mux.Handle("PATCH /auth/password", s.private(&rl.AuthMutate, s.updatePassword))
	mux.Handle("DELETE /auth/delete", s.private(&rl.AuthMutate, s.deleteAccount))

	// Recovery (unauthenticated)
	mux.Handle("POST /auth/forgot-username", s.public(&rl.ForgotUsername, s.forgotUsername))
	mux.Handle("POST /auth/password-reset/request", s.public(&rl.PasswordResetRequest, s.requestPasswordReset))
	mux.Handle("POST /auth/password-reset/verify", s.public(&rl.PasswordResetVerify, s.verifyPasswordReset))

	// Email
	mux.Handle("POST /auth/email", s.private(&rl.EmailMutate, s.addEmail))
	mux.Handle("PATCH /auth/email", s.private(&rl.EmailMutate, s.changeEmail))
	mux.Handle("POST /auth/email/resend", s.private(&rl.EmailMutate, s.resendEmail))
	mux.Handle("POST /auth/email/confirm", s.private(&rl.EmailMutate, s.confirmEmail))

	// History
	mux.Handle("GET /history/sessions", s.private(nil, s.history))

	// Workouts
	mux.Handle("POST /workouts", s.private(nil, s.createWorkout))
	mux.Handle("GET /workouts", s.private(nil, s.listWorkouts))
	mux.Handle("PUT /workouts/order", s.private(nil, s.reorderWorkouts)) // additive
	mux.Handle("GET /workouts/{id}", s.private(nil, s.getWorkout))
	mux.Handle("DELETE /workouts/{id}", s.private(nil, s.deleteWorkout))
	mux.Handle("PATCH /workouts/{id}", s.private(nil, s.patchWorkout)) // additive
	mux.Handle("PATCH /workouts/{id}/{field}", s.private(nil, s.patchWorkoutField))

	// Exercises
	const ex = "/workouts/{workoutId}/exercises"
	mux.Handle("POST "+ex, s.private(nil, s.createExercise))
	mux.Handle("GET "+ex, s.private(nil, s.listExercises))
	mux.Handle("PUT "+ex+"/order", s.private(nil, s.reorderExercises)) // additive
	mux.Handle("GET "+ex+"/{id}", s.private(nil, s.getExercise))
	mux.Handle("DELETE "+ex+"/{id}", s.private(nil, s.deleteExercise))
	mux.Handle("PATCH "+ex+"/{id}", s.private(nil, s.patchExercise)) // additive
	mux.Handle("PATCH "+ex+"/{id}/{field}", s.private(nil, s.patchExerciseField))
	mux.Handle("GET "+ex+"/{id}/averages", s.private(nil, s.averages(false)))
	mux.Handle("GET "+ex+"/{id}/averages/all", s.private(nil, s.averages(true)))

	// Workout sessions
	const ws = "/workouts/{workoutId}/sessions"
	mux.Handle("POST "+ws, s.private(nil, s.createSession))
	mux.Handle("GET "+ws+"/{id}", s.private(nil, s.getSession))
	mux.Handle("DELETE "+ws+"/{id}", s.private(nil, s.deleteSession))
	mux.Handle("PATCH "+ws+"/{id}/{field}", s.private(nil, s.patchSessionField))

	// Exercise sessions
	const es = ws + "/{sessionId}/exerciseSessions"
	mux.Handle("POST "+es, s.private(nil, s.createExerciseSession))
	mux.Handle("POST "+es+"/bulk", s.private(nil, s.bulkCreateExerciseSessions)) // additive
	mux.Handle("GET "+es, s.private(nil, s.listExerciseSessions))
	mux.Handle("GET "+es+"/{id}", s.private(nil, s.getExerciseSession))
	mux.Handle("DELETE "+es+"/{id}", s.private(nil, s.deleteExerciseSession))
	mux.Handle("GET "+es+"/{id}/previous-set-count", s.private(nil, s.previousSetCount))

	// Set sessions
	const sets = es + "/{exerciseSessionId}/setSessions"
	mux.Handle("POST "+sets, s.private(nil, s.createSet))
	mux.Handle("GET "+sets, s.private(nil, s.listSets))
	mux.Handle("GET "+sets+"/{id}", s.private(nil, s.getSet))
	mux.Handle("DELETE "+sets+"/{id}", s.private(nil, s.deleteSet))
	mux.Handle("PATCH "+sets+"/{id}", s.private(nil, s.patchSet)) // additive
	mux.Handle("PATCH "+sets+"/{id}/{field}", s.private(nil, s.patchSetField))

	// Everything else -- including a known path with the wrong method -- gets
	// a JSON 404 rather than net/http's plain text, which the client can't read.
	mux.Handle("/", httpx.NotFoundHandler())
}
