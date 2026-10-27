package model

import "github.com/dtmesa/gravappbe/internal/jsonx"

// Response bodies that are not entities.

type StatusResponse struct {
	Status string `json:"status"`
}

// RegisterResponse adds token to the original {id, username}. The current app
// ignores the register response and logs in separately; returning a token
// lets a future build skip that second request and its bcrypt verify.
type RegisterResponse struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

type TokenResponse struct {
	Token string `json:"token"`
}

type UsernameResponse struct {
	Username string `json:"username"`
}

type MeResponse struct {
	Username       string  `json:"username"`
	Email          *string `json:"email"`
	EmailConfirmed bool    `json:"emailConfirmed"`
	PendingEmail   *string `json:"pendingEmail"`
}

type PendingEmailResponse struct {
	PendingEmail string `json:"pendingEmail"`
}

type EmailConfirmedResponse struct {
	Email          string `json:"email"`
	EmailConfirmed bool   `json:"emailConfirmed"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

type CountResponse struct {
	Count int `json:"count"`
}

// Averages always carries all four keys. The client filters on `!== null`, so a
// missing key would pass the filter and then render NaN.
type Averages struct {
	Weight   *float64 `json:"weight"`
	Reps     *float64 `json:"reps"`
	Duration *float64 `json:"duration"`
	Distance *float64 `json:"distance"`
}

// The /history/sessions projection. Prisma served it from one query with
// nested includes; it is assembled by hand here, with the same wire shape.

type NamedRef struct {
	Name string `json:"name"`
}

type HistoryExerciseSession struct {
	ID               int             `json:"id"`
	Order            int             `json:"order"`
	WorkoutSessionID int             `json:"workoutSessionId"`
	ExerciseID       int             `json:"exerciseId"`
	CreatedAt        jsonx.Timestamp `json:"createdAt"`
	Exercise         NamedRef        `json:"exercise"`
	Sets             []SetSession    `json:"sets"`
}

// HistorySession must keep the top-level workoutId: the History screen builds
// its delete and navigation paths from it, not from workout.
type HistorySession struct {
	ID        int                      `json:"id"`
	Date      jsonx.Timestamp          `json:"date"`
	UserID    int                      `json:"userId"`
	WorkoutID int                      `json:"workoutId"`
	CreatedAt jsonx.Timestamp          `json:"createdAt"`
	Workout   NamedRef                 `json:"workout"`
	Exercises []HistoryExerciseSession `json:"exercises"`
}
