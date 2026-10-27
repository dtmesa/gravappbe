// Package model holds the storage and wire types.
//
// One struct serves both DynamoDB (dynamodbav tags) and JSON (json tags), so
// the two tag sets have deliberately different null rules:
//
//   - JSON: nullable scalars have NO omitempty. They must serialize as an
//     explicit null -- the client filters on `value !== null`, so a missing key
//     breaks the UI rather than reading as null.
//   - DynamoDB: nullable scalars use omitempty, so a nil pointer is left off the
//     item rather than stored as NULL. A pointer to zero is still stored; only a
//     nil pointer is omitted.
//
// Never put omitempty on a non-pointer number or bool in either tag set:
// `omitempty` treats 0 and false as empty, and a fresh set's 0 weight is a real
// value the UI renders.
package model

import "github.com/dtmesa/gravappbe/internal/jsonx"

type User struct {
	ID             int             `dynamodbav:"id"`
	Username       string          `dynamodbav:"username"`
	Password       string          `dynamodbav:"password"`
	Email          *string         `dynamodbav:"email,omitempty"`
	EmailConfirmed bool            `dynamodbav:"emailConfirmed"`
	TokenVersion   int             `dynamodbav:"tokenVersion"`
	CreatedAt      jsonx.Timestamp `dynamodbav:"createdAt"`
}

type Workout struct {
	ID          int             `json:"id" dynamodbav:"id"`
	Name        string          `json:"name" dynamodbav:"name"`
	Description *string         `json:"description" dynamodbav:"description,omitempty"`
	Order       int             `json:"order" dynamodbav:"order"`
	UserID      int             `json:"userId" dynamodbav:"userId"`
	CreatedAt   jsonx.Timestamp `json:"createdAt" dynamodbav:"createdAt"`
}

type Exercise struct {
	ID          int             `json:"id" dynamodbav:"id"`
	Order       int             `json:"order" dynamodbav:"order"`
	Name        string          `json:"name" dynamodbav:"name"`
	Description *string         `json:"description" dynamodbav:"description,omitempty"`
	WorkoutID   int             `json:"workoutId" dynamodbav:"workoutId"`
	IsWeight    bool            `json:"isWeight" dynamodbav:"isWeight"`
	IsDuration  bool            `json:"isDuration" dynamodbav:"isDuration"`
	IsDistance  bool            `json:"isDistance" dynamodbav:"isDistance"`
	IsReps      bool            `json:"isReps" dynamodbav:"isReps"`
	CreatedAt   jsonx.Timestamp `json:"createdAt" dynamodbav:"createdAt"`
}

type WorkoutSession struct {
	ID        int             `json:"id" dynamodbav:"id"`
	Date      jsonx.Timestamp `json:"date" dynamodbav:"date"`
	UserID    int             `json:"userId" dynamodbav:"userId"`
	WorkoutID int             `json:"workoutId" dynamodbav:"workoutId"`
	CreatedAt jsonx.Timestamp `json:"createdAt" dynamodbav:"createdAt"`

	// Populated only where the original route included the workout; omitted
	// from the JSON otherwise, as Prisma's `include` did.
	Workout *Workout `json:"workout,omitempty" dynamodbav:"-"`
}

// ExerciseSession is one exercise logged within a workout session.
type ExerciseSession struct {
	ID               int             `json:"id" dynamodbav:"id"`
	Order            int             `json:"order" dynamodbav:"order"`
	WorkoutSessionID int             `json:"workoutSessionId" dynamodbav:"workoutSessionId"`
	ExerciseID       int             `json:"exerciseId" dynamodbav:"exerciseId"`
	CreatedAt        jsonx.Timestamp `json:"createdAt" dynamodbav:"createdAt"`

	// Always an array on the wire: the client calls dto.sets.map() unguarded,
	// so null would throw. Handlers must set it to a non-nil slice.
	Sets []SetSession `json:"sets" dynamodbav:"-"`

	// Denormalized from the parent session. /averages and /previous-set-count
	// query across sessions by exerciseId, which would otherwise be a scan.
	UserID      int             `json:"-" dynamodbav:"userId"`
	WorkoutID   int             `json:"-" dynamodbav:"workoutId"`
	SessionDate jsonx.Timestamp `json:"-" dynamodbav:"sessionDate"`

	Aggregates
}

// Aggregates are running per-metric totals over this exercise session's sets,
// maintained with atomic ADDs on every set write, so /averages needs no
// per-session set query. See averages.go for how they are consumed.
//
// SetCount is a pointer so a row written before aggregates existed (attribute
// absent) is distinguishable from one with zero sets; such rows are backfilled
// from their sets on first touch. It deliberately has no omitempty, so a real
// zero is always stored.
type Aggregates struct {
	SetCount      *int    `json:"-" dynamodbav:"setCount"`
	WeightSum     float64 `json:"-" dynamodbav:"weightSum"`
	WeightCount   int     `json:"-" dynamodbav:"weightCount"`
	RepsSum       float64 `json:"-" dynamodbav:"repsSum"`
	RepsCount     int     `json:"-" dynamodbav:"repsCount"`
	DurationSum   float64 `json:"-" dynamodbav:"durationSum"`
	DurationCount int     `json:"-" dynamodbav:"durationCount"`
	DistanceSum   float64 `json:"-" dynamodbav:"distanceSum"`
	DistanceCount int     `json:"-" dynamodbav:"distanceCount"`
}

// HasAggregates reports whether the row carries maintained aggregates.
func (a Aggregates) HasAggregates() bool { return a.SetCount != nil }

// Count returns the set count, treating missing aggregates as zero.
func (a Aggregates) Count() int {
	if a.SetCount == nil {
		return 0
	}

	return *a.SetCount
}

type SetSession struct {
	ID                int             `json:"id" dynamodbav:"id"`
	Weight            *float64        `json:"weight" dynamodbav:"weight,omitempty"`
	Reps              *int            `json:"reps" dynamodbav:"reps,omitempty"`
	Duration          *int            `json:"duration" dynamodbav:"duration,omitempty"`
	Distance          *float64        `json:"distance" dynamodbav:"distance,omitempty"`
	ExerciseSessionID int             `json:"exerciseSessionId" dynamodbav:"exerciseSessionId"`
	CreatedAt         jsonx.Timestamp `json:"createdAt" dynamodbav:"createdAt"`
}

// Metric names, which are also the attribute and JSON names on SetSession.
const (
	MetricWeight   = "weight"
	MetricReps     = "reps"
	MetricDuration = "duration"
	MetricDistance = "distance"
)

// Metrics lists every set metric, in a fixed order.
var Metrics = []string{MetricWeight, MetricReps, MetricDuration, MetricDistance}

// Value returns the named metric as a float, or nil when it is null.
func (s SetSession) Value(metric string) *float64 {
	switch metric {
	case MetricWeight:
		return s.Weight
	case MetricDistance:
		return s.Distance
	case MetricReps:
		return intToFloat(s.Reps)
	case MetricDuration:
		return intToFloat(s.Duration)
	}

	return nil
}

func intToFloat(p *int) *float64 {
	if p == nil {
		return nil
	}

	f := float64(*p)

	return &f
}
