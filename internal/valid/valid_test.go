package valid

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/dtmesa/gravappbe/internal/httpx"
)

func issues(err error) []string {
	var e *httpx.Error
	if !errors.As(err, &e) {
		return nil
	}

	var out []string
	for _, i := range e.Issues {
		out = append(out, i.Path)
	}

	return out
}

func TestRequestValidation(t *testing.T) {
	cases := []struct {
		name string
		req  Validator
		want []string // failing paths; nil means valid
	}{
		{"register ok", Register{"bob", "password1"}, nil},
		{"username too short", Register{"bo", "password1"}, []string{"username"}},
		{"username 20 is ok", Register{strings.Repeat("a", 20), "password1"}, nil},
		{"username 21 too long", Register{strings.Repeat("a", 21), "password1"}, []string{"username"}},
		{"username padded", Register{" bob", "password1"}, []string{"username"}},
		{"password short and padded", Register{"bob", " pass"}, []string{"password", "password"}},
		{"password 32 ok", Register{"bob", strings.Repeat("p", 32)}, nil},
		{"password 33 too long", Register{"bob", strings.Repeat("p", 33)}, []string{"password"}},
		{"login empty", Login{"", " "}, []string{"username", "password"}},
		{"email ok", EmailOnly{"a@b"}, nil},
		{"email missing", EmailOnly{""}, []string{"email"}},
		{"email no at", EmailOnly{"ab"}, []string{"email"}},
		{"email two ats", EmailOnly{"a@b@c"}, []string{"email"}},
		{"email trailing at", EmailOnly{"ab@"}, []string{"email"}},
		{"code ok", ConfirmEmail{"012345"}, nil},
		{"code short", ConfirmEmail{"12345"}, []string{"code"}},
		{"code letters", ConfirmEmail{"12345a"}, []string{"code"}},
		{"name ok", CreateNamed{"Push day"}, nil},
		{"name blank", CreateNamed{"  "}, []string{"name", "name"}},
		{"name 75 ok", CreateNamed{strings.Repeat("n", 75)}, nil},
		{"name 76 too long", CreateNamed{strings.Repeat("n", 76)}, []string{"name"}},
		{"exercise id zero", CreateExerciseSession{0}, []string{"exerciseId"}},
		{"reorder duplicate", Reorder{[]int{1, 1}}, []string{"ids"}},
		{"reorder ok", Reorder{[]int{3, 1, 2}}, nil},
		{"bulk empty", BulkExerciseSessions{nil}, []string{"exerciseIds"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := issues(c.req.Validate()); !slices.Equal(got, c.want) {
				t.Errorf("issues = %v, want %v", got, c.want)
			}
		})
	}
}

// Length is counted in UTF-16 code units, like .NET and the client's JS.
func TestLengthCountsUTF16Units(t *testing.T) {
	// "😀" is one rune but two UTF-16 units; 10 of them is 20 units.
	if err := (Register{strings.Repeat("😀", 10), "password1"}).Validate(); err != nil {
		t.Errorf("20 units rejected: %v", err)
	}

	if err := (Register{strings.Repeat("😀", 10) + "a", "password1"}).Validate(); err == nil {
		t.Error("21 units accepted")
	}
}

func TestLegacyFieldPatch(t *testing.T) {
	p, err := WorkoutField("order", []byte(`{"order":3}`))
	if err != nil || p.Order == nil || *p.Order != 3 {
		t.Fatalf("order patch = %+v, %v", p, err)
	}

	for name, c := range map[string]struct {
		field, body, path string
	}{
		"unsupported field":        {"userId", `{"userId":1}`, "userId"},
		"missing value":            {"name", `{"other":"x"}`, "name"},
		"body not object":          {"name", `"x"`, "name"},
		"key is case-sensitive":    {"name", `{"Name":"x"}`, "name"},
		"name empty":               {"name", `{"name":""}`, "name"},
		"name padded":              {"name", `{"name":"x "}`, "name"},
		"order fractional":         {"order", `{"order":1.5}`, "order"},
		"order exponent":           {"order", `{"order":1e2}`, "order"},
		"order negative":           {"order", `{"order":-1}`, "order"},
		"order string":             {"order", `{"order":"3"}`, "order"},
		"description not string":   {"description", `{"description":5}`, "description"},
		"description null":         {"description", `{"description":null}`, "description"},
		"description 501 too long": {"description", `{"description":"` + strings.Repeat("d", 501) + `"}`, "description"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := WorkoutField(c.field, []byte(c.body))
			if got := issues(err); !slices.Equal(got, []string{c.path}) {
				t.Errorf("issues = %v, want [%s]", got, c.path)
			}
		})
	}
}

func TestSetPatchRejectsNull(t *testing.T) {
	// The client sends null when a field is cleared. Persisting it would make
	// the input disappear, so it is rejected, as it always was.
	if _, err := SetField("weight", []byte(`{"weight":null}`)); err == nil {
		t.Error("null weight accepted")
	}

	p, err := SetField("weight", []byte(`{"weight":0}`))
	if err != nil || p.Weight == nil || *p.Weight != 0 {
		t.Errorf("zero weight = %+v, %v", p, err)
	}

	p, err = SetField("distance", []byte(`{"distance":2.5}`))
	if err != nil || *p.Distance != 2.5 {
		t.Errorf("decimal distance = %+v, %v", p, err)
	}

	if _, err := SetField("reps", []byte(`{"reps":2.5}`)); err == nil {
		t.Error("fractional reps accepted")
	}
}

func TestExerciseFlags(t *testing.T) {
	p, err := ExerciseField("isWeight", []byte(`{"isWeight":false}`))
	if err != nil || p.IsWeight == nil || *p.IsWeight {
		t.Errorf("flag = %+v, %v", p, err)
	}

	if _, err := ExerciseField("isWeight", []byte(`{"isWeight":"true"}`)); err == nil {
		t.Error("string flag accepted")
	}
}

func TestSparsePatch(t *testing.T) {
	p, err := WorkoutSparse([]byte(`{"name":"Legs","order":2}`))
	if err != nil || *p.Name != "Legs" || *p.Order != 2 || p.Description != nil {
		t.Fatalf("sparse = %+v, %v", p, err)
	}

	if _, err := WorkoutSparse([]byte(`{}`)); err == nil {
		t.Error("empty sparse body accepted")
	}

	_, err = WorkoutSparse([]byte(`{"name":"","bogus":1}`))
	if got := issues(err); len(got) != 2 {
		t.Errorf("want both failures collected, got %v", got)
	}
}

func TestSessionDate(t *testing.T) {
	if _, err := SessionDate("date", []byte(`{"date":"2026-01-02T03:04:05.000Z"}`)); err != nil {
		t.Errorf("valid date rejected: %v", err)
	}

	var e *httpx.Error

	if _, err := SessionDate("date", []byte(`{}`)); !errors.As(err, &e) || e.Code != "INVALID_BODY" {
		t.Errorf("missing date = %v, want INVALID_BODY", err)
	}

	if _, err := SessionDate("date", []byte(`{"date":"nope"}`)); !errors.As(err, &e) || e.Code != "VALIDATION_ERROR" {
		t.Errorf("bad date = %v, want VALIDATION_ERROR", err)
	}

	if _, err := SessionDate("workoutId", []byte(`{"workoutId":1}`)); !errors.As(err, &e) || e.Code != "VALIDATION_ERROR" {
		t.Errorf("unsupported field = %v", err)
	}
}
