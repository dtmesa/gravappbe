// Package httpx is the HTTP plumbing shared by every handler: the error type,
// the error envelope, JSON reading and writing, and the adapter that lets
// handlers return errors instead of writing failure responses themselves.
package httpx

import (
	"fmt"
	"net/http"
	"strconv"
)

// Error carries an HTTP status and the machine-readable code the client
// branches on. The app reads only `response.data.error` -- never the HTTP
// status -- so Code is the real contract and Status is for everyone else.
type Error struct {
	Status     int
	Code       string
	Message    string
	RetryAfter int
	Issues     []Issue
}

// Issue is one validation failure. Nothing in the client reads these; they
// exist so a 400 is debuggable.
type Issue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Message)
	}

	return fmt.Sprintf("%d %s", e.Status, e.Code)
}

// New builds an error with an explicit status and code.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// NotFound matches AppError.NotFound: "<entity> not found" with an
// entity-specific code such as WORKOUT_NOT_FOUND.
func NotFound(entity, code string) *Error {
	return New(http.StatusNotFound, code, entity+" not found")
}

// Unauthorized is the response for a missing, invalid or invalidated token.
// The Express backend used the code TOKEN for all three, and so does this.
func Unauthorized() *Error {
	return New(http.StatusUnauthorized, "TOKEN", "Token is missing")
}

// TooManyRequests carries the seconds until the current window resets.
func TooManyRequests(retryAfter int) *Error {
	return &Error{Status: http.StatusTooManyRequests, Code: "RATE_LIMITED", Message: "Too many requests", RetryAfter: retryAfter}
}

// Invalid is a single-issue 400 VALIDATION_ERROR.
func Invalid(path, message string) *Error {
	return Validation(Issue{Path: path, Message: message})
}

// Validation is a 400 VALIDATION_ERROR carrying every failure found.
func Validation(issues ...Issue) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: "Validation failed", Issues: issues}
}

// errorBody is the wire envelope. Issues is omitted rather than written as
// null, so every non-validation error is a bare {"error":"CODE"}.
type errorBody struct {
	Error  string  `json:"error"`
	Issues []Issue `json:"issues,omitempty"`
}

func writeError(w http.ResponseWriter, e *Error) {
	if e.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(e.RetryAfter))
	}

	WriteJSON(w, e.Status, errorBody{Error: e.Code, Issues: e.Issues})
}
