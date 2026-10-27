package valid

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strconv"

	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/jsonx"
)

// The live PATCH routes are /…/{id}/{field} with a body whose single key must
// equal {field} -- a Zod discriminated union in the original. That shape can't
// be described in OpenAPI, so each entity also accepts a sparse body
// ({name?, order?, …}) on PATCH /…/{id}. Both decode into the same patch type
// and share one set of rules; the allowlists here are also what keep the raw
// {field} route value out of any DynamoDB expression.

type WorkoutPatch struct {
	Name        *string
	Description *string
	Order       *int
}

type ExercisePatch struct {
	Name        *string
	Description *string
	Order       *int
	IsWeight    *bool
	IsDuration  *bool
	IsDistance  *bool
	IsReps      *bool
}

// SetPatch fields reject null. The C# service rejected it too, and that is
// load-bearing: a null metric means "untracked" to the client, which hides
// the input entirely, so clearing a field must not persist null.
type SetPatch struct {
	Weight   *float64
	Reps     *int
	Duration *int
	Distance *float64
}

type fieldRule[P any] func(p *P, raw json.RawMessage, field string) error

var workoutRules = map[string]fieldRule[WorkoutPatch]{
	"order": func(p *WorkoutPatch, raw json.RawMessage, f string) (err error) {
		p.Order, err = integer(raw, f, 0)
		return
	},
	"description": func(p *WorkoutPatch, raw json.RawMessage, f string) (err error) {
		p.Description, err = text(raw, f, 500)
		return
	},
	"name": func(p *WorkoutPatch, raw json.RawMessage, f string) (err error) { p.Name, err = name(raw, f); return },
}

var exerciseRules = map[string]fieldRule[ExercisePatch]{
	"order": func(p *ExercisePatch, raw json.RawMessage, f string) (err error) {
		p.Order, err = integer(raw, f, 0)
		return
	},
	"description": func(p *ExercisePatch, raw json.RawMessage, f string) (err error) {
		p.Description, err = text(raw, f, 500)
		return
	},
	"name": func(p *ExercisePatch, raw json.RawMessage, f string) (err error) { p.Name, err = name(raw, f); return },
	"isWeight": func(p *ExercisePatch, raw json.RawMessage, f string) (err error) {
		p.IsWeight, err = flag(raw, f)
		return
	},
	"isDuration": func(p *ExercisePatch, raw json.RawMessage, f string) (err error) {
		p.IsDuration, err = flag(raw, f)
		return
	},
	"isDistance": func(p *ExercisePatch, raw json.RawMessage, f string) (err error) {
		p.IsDistance, err = flag(raw, f)
		return
	},
	"isReps": func(p *ExercisePatch, raw json.RawMessage, f string) (err error) {
		p.IsReps, err = flag(raw, f)
		return
	},
}

// reps and duration back integer fields, so they must be integers; the Zod
// schema allowed any non-negative number for duration, which Prisma then
// rejected at write time with a 500.
var setRules = map[string]fieldRule[SetPatch]{
	"reps": func(p *SetPatch, raw json.RawMessage, f string) (err error) { p.Reps, err = integer(raw, f, 0); return },
	"duration": func(p *SetPatch, raw json.RawMessage, f string) (err error) {
		p.Duration, err = integer(raw, f, 0)
		return
	},
	"weight": func(p *SetPatch, raw json.RawMessage, f string) (err error) {
		p.Weight, err = decimal(raw, f, 0)
		return
	},
	"distance": func(p *SetPatch, raw json.RawMessage, f string) (err error) {
		p.Distance, err = decimal(raw, f, 0)
		return
	},
}

func WorkoutField(field string, body []byte) (WorkoutPatch, error) {
	return single(workoutRules, field, body)
}

func WorkoutSparse(body []byte) (WorkoutPatch, error) { return sparse(workoutRules, body) }

func ExerciseField(field string, body []byte) (ExercisePatch, error) {
	return single(exerciseRules, field, body)
}

func ExerciseSparse(body []byte) (ExercisePatch, error) { return sparse(exerciseRules, body) }

func SetField(field string, body []byte) (SetPatch, error) { return single(setRules, field, body) }

func SetSparse(body []byte) (SetPatch, error) { return sparse(setRules, body) }

// single handles the legacy /{field} shape. An unsupported field is rejected
// before the body is looked at, as the original switch did.
func single[P any](rules map[string]fieldRule[P], field string, body []byte) (P, error) {
	var p P

	rule, ok := rules[field]
	if !ok {
		return p, httpx.Invalid(field, "Unsupported field '"+field+"'")
	}

	obj, isObject := object(body)

	raw, present := obj[field]
	if !isObject || !present {
		return p, httpx.Invalid(field, "Missing value for field '"+field+"'")
	}

	return p, rule(&p, raw, field)
}

// sparse handles the {a?, b?, …} shape, collecting every failure.
func sparse[P any](rules map[string]fieldRule[P], body []byte) (P, error) {
	var p P

	obj, isObject := object(body)
	if !isObject {
		return p, httpx.Invalid("body", "Request body must be a JSON object")
	}

	if len(obj) == 0 {
		return p, httpx.Invalid("body", "At least one field is required")
	}

	var c collector

	for field, raw := range obj {
		rule, ok := rules[field]
		if !ok {
			c.add(field, "Unsupported field '"+field+"'")
			continue
		}

		if err := rule(&p, raw, field); err != nil {
			var e *httpx.Error
			if errors.As(err, &e) {
				c = append(c, e.Issues...)
			}
		}
	}

	return p, c.err()
}

// SessionDate reads the single patchable field on a workout session. A missing
// date is 400 INVALID_BODY rather than a validation error: the Zod schema made
// date optional and the handler then rejected its absence separately.
func SessionDate(field string, body []byte) (jsonx.Timestamp, error) {
	if field != "date" {
		return jsonx.Timestamp{}, httpx.Invalid(field, "Unsupported field '"+field+"'")
	}

	obj, isObject := object(body)

	raw, present := obj["date"]
	if !isObject || !present {
		return jsonx.Timestamp{}, httpx.New(400, "INVALID_BODY", "Missing value for field")
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return jsonx.Timestamp{}, httpx.Invalid("date", "'date' must be a date")
	}

	ts, err := jsonx.Parse(s)
	if err != nil {
		return jsonx.Timestamp{}, httpx.Invalid("date", "'date' must be a date")
	}

	return ts, nil
}

func object(body []byte) (map[string]json.RawMessage, bool) {
	if !bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		return nil, false
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, false
	}

	return obj, true
}

func text(raw json.RawMessage, field string, maxLength int) (*string, error) {
	var s string
	if !isString(raw) || json.Unmarshal(raw, &s) != nil {
		return nil, httpx.Invalid(field, "'"+field+"' must be a string")
	}

	if length(s) > maxLength {
		return nil, httpx.Invalid(field, "'"+field+"' must be at most "+strconv.Itoa(maxLength)+" characters")
	}

	return &s, nil
}

func name(raw json.RawMessage, field string) (*string, error) {
	s, err := text(raw, field, 75)
	if err != nil {
		return nil, err
	}

	if length(*s) < 1 {
		return nil, httpx.Invalid(field, "'"+field+"' must not be empty")
	}

	if padded(*s) {
		return nil, httpx.Invalid(field, "Name cannot start or end with spaces")
	}

	return s, nil
}

// integer accepts only an integral JSON number in 32-bit range, as
// JsonElement.TryGetInt32 did -- 1.5 and 1e2 are rejected.
func integer(raw json.RawMessage, field string, minimum int) (*int, error) {
	n, err := strconv.ParseInt(string(bytes.TrimSpace(raw)), 10, 32)
	if err != nil {
		return nil, httpx.Invalid(field, "'"+field+"' must be an integer")
	}

	if int(n) < minimum {
		return nil, httpx.Invalid(field, "'"+field+"' must be at least "+strconv.Itoa(minimum))
	}

	v := int(n)

	return &v, nil
}

func decimal(raw json.RawMessage, field string, minimum float64) (*float64, error) {
	var f float64
	if !isNumber(raw) || json.Unmarshal(raw, &f) != nil || math.IsInf(f, 0) {
		return nil, httpx.Invalid(field, "'"+field+"' must be a number")
	}

	if f < minimum {
		return nil, httpx.Invalid(field, "'"+field+"' must be at least "+strconv.FormatFloat(minimum, 'g', -1, 64))
	}

	return &f, nil
}

func flag(raw json.RawMessage, field string) (*bool, error) {
	switch string(bytes.TrimSpace(raw)) {
	case "true":
		v := true
		return &v, nil
	case "false":
		v := false
		return &v, nil
	}

	return nil, httpx.Invalid(field, "'"+field+"' must be a boolean")
}

func isString(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '"'
}

func isNumber(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && (t[0] == '-' || (t[0] >= '0' && t[0] <= '9'))
}
