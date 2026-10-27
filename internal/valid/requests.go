// Package valid holds request bodies and their validation, ported from
// Validation/Validators.cs (itself a port of the Zod schemas). Messages are
// carried over where the original supplied one. The client reads only the
// top-level error code; the issues exist to make a 400 debuggable.
package valid

import (
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/jsonx"
)

// Validator is implemented by every request body.
type Validator interface {
	Validate() error
}

// collector accumulates every failure, as FluentValidation did, rather than
// stopping at the first.
type collector []httpx.Issue

func (c *collector) add(path, message string) {
	*c = append(*c, httpx.Issue{Path: path, Message: message})
}

func (c collector) err() error {
	if len(c) == 0 {
		return nil
	}

	return httpx.Validation(c...)
}

// length counts UTF-16 code units, matching both .NET's string.Length and the
// client's JS .length.
func length(s string) int {
	return len(utf16.Encode([]rune(s)))
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }

func padded(s string) bool { return s != strings.TrimSpace(s) }

func (c *collector) notEmpty(path, value, message string) {
	if blank(value) {
		c.add(path, message)
	}
}

func (c *collector) password(path, v string) {
	if length(v) < 8 {
		c.add(path, "Password must be at least 8 characters.")
	}

	if length(v) > 32 {
		c.add(path, "Password must be fewer than 32 characters.")
	}

	if padded(v) {
		c.add(path, "Password cannot start or end with spaces")
	}
}

func (c *collector) username(path, v string) {
	if length(v) < 3 {
		c.add(path, "Username must be at least 3 characters")
	}

	if length(v) > 20 {
		c.add(path, "Username must be fewer than 20 characters")
	}

	if padded(v) {
		c.add(path, "Username cannot start or end with spaces")
	}
}

// entityName is shared by workout and exercise names.
func (c *collector) entityName(path, v string) {
	if blank(v) {
		c.add(path, "Name must not be empty")
	}

	if length(v) > 75 {
		c.add(path, "Name must be at most 75 characters")
	}

	if padded(v) {
		c.add(path, "Name cannot start or end with spaces")
	}
}

func (c *collector) email(path, v string) {
	if blank(v) {
		c.add(path, "Email is required")
		return
	}

	if !looksLikeEmail(v) {
		c.add(path, "Enter a valid email address")
	}

	if length(v) > 254 {
		c.add(path, "Email is too long")
	}
}

// looksLikeEmail is FluentValidation's default (AspNetCoreCompatible) check:
// exactly one @, neither first nor last. Deliberately permissive -- the real
// proof of an address is the confirmation code.
func looksLikeEmail(v string) bool {
	at := strings.IndexByte(v, '@')
	return at > 0 && at != len(v)-1 && at == strings.LastIndexByte(v, '@')
}

var sixDigits = regexp.MustCompile(`^\d{6}$`)

func (c *collector) code(path, v string) {
	if !sixDigits.MatchString(v) {
		c.add(path, "Enter the 6-digit code")
	}
}

type Register struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r Register) Validate() error {
	var c collector
	c.username("username", r.Username)
	c.password("password", r.Password)

	return c.err()
}

type Login struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r Login) Validate() error {
	var c collector
	c.notEmpty("username", r.Username, "Username is required")
	c.notEmpty("password", r.Password, "Password is required")

	return c.err()
}

type UpdateUsername struct {
	NewUsername string `json:"newUsername"`
	Password    string `json:"password"`
}

func (r UpdateUsername) Validate() error {
	var c collector
	c.username("newUsername", r.NewUsername)
	c.notEmpty("password", r.Password, "Password is required")

	return c.err()
}

type UpdatePassword struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (r UpdatePassword) Validate() error {
	var c collector
	c.notEmpty("currentPassword", r.CurrentPassword, "Current password is required")
	c.password("newPassword", r.NewPassword)

	return c.err()
}

type DeleteAccount struct {
	Password string `json:"password"`
}

func (r DeleteAccount) Validate() error {
	var c collector
	c.notEmpty("password", r.Password, "Password is required")

	return c.err()
}

type EmailOnly struct {
	Email string `json:"email"`
}

func (r EmailOnly) Validate() error {
	var c collector
	c.email("email", r.Email)

	return c.err()
}

type VerifyPasswordReset struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"newPassword"`
}

func (r VerifyPasswordReset) Validate() error {
	var c collector
	c.email("email", r.Email)
	c.code("code", r.Code)
	c.password("newPassword", r.NewPassword)

	return c.err()
}

type AddEmail struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r AddEmail) Validate() error {
	var c collector
	c.email("email", r.Email)
	c.notEmpty("password", r.Password, "Password is required")

	return c.err()
}

type ConfirmEmail struct {
	Code string `json:"code"`
}

func (r ConfirmEmail) Validate() error {
	var c collector
	c.code("code", r.Code)

	return c.err()
}

type ChangeEmail struct {
	NewEmail string `json:"newEmail"`
	Password string `json:"password"`
}

func (r ChangeEmail) Validate() error {
	var c collector
	c.email("newEmail", r.NewEmail)
	c.notEmpty("password", r.Password, "Password is required")

	return c.err()
}

// CreateNamed is the create body for both workouts and exercises.
type CreateNamed struct {
	Name string `json:"name"`
}

func (r CreateNamed) Validate() error {
	var c collector
	c.entityName("name", r.Name)

	return c.err()
}

type CreateWorkoutSession struct {
	Date *jsonx.Timestamp `json:"date"`
}

func (CreateWorkoutSession) Validate() error { return nil }

type CreateExerciseSession struct {
	ExerciseID int `json:"exerciseId"`
}

func (r CreateExerciseSession) Validate() error {
	var c collector

	if r.ExerciseID <= 0 {
		c.add("exerciseId", "exerciseId must be greater than 0")
	}

	return c.err()
}

// BulkExerciseSessions creates several exercise sessions in one call.
type BulkExerciseSessions struct {
	ExerciseIDs []int `json:"exerciseIds"`
}

func (r BulkExerciseSessions) Validate() error {
	var c collector

	if len(r.ExerciseIDs) == 0 {
		c.add("exerciseIds", "exerciseIds must not be empty")
	}

	// One transaction holds at most 100 writes.
	if len(r.ExerciseIDs) > 100 {
		c.add("exerciseIds", "exerciseIds must have at most 100 entries")
	}

	for _, id := range r.ExerciseIDs {
		if id <= 0 {
			c.add("exerciseIds", "every exerciseId must be greater than 0")
			break
		}
	}

	return c.err()
}

// Reorder assigns order = index to each listed id.
type Reorder struct {
	IDs []int `json:"ids"`
}

func (r Reorder) Validate() error {
	var c collector

	if len(r.IDs) == 0 {
		c.add("ids", "ids must not be empty")
	}

	if len(r.IDs) > 100 {
		c.add("ids", "ids must have at most 100 entries")
	}

	seen := make(map[int]bool, len(r.IDs))

	for _, id := range r.IDs {
		if id <= 0 {
			c.add("ids", "every id must be greater than 0")
			break
		}

		if seen[id] {
			c.add("ids", "ids must be unique")
			break
		}

		seen[id] = true
	}

	return c.err()
}
