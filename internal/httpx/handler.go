package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
)

// HandlerFunc is the signature every route handler uses. Go has no exceptions,
// so where the C# service threw AppError from deep inside a repository and
// caught it in middleware, handlers here simply return the error and Handle
// renders it -- one place, instead of a write-and-return in every handler.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handle adapts a HandlerFunc into an http.Handler.
func Handle(fn HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			RenderError(w, r, err)
		}
	})
}

// RenderError writes err as the error envelope. Anything that is not an
// *Error is logged and becomes an opaque 500, so internals never leak.
func RenderError(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if errors.As(err, &e) {
		writeError(w, e)
		return
	}

	slog.ErrorContext(r.Context(), "unhandled error", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, New(http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", ""))
}

// Recover turns a panic into the same 500 envelope, rather than letting
// net/http drop the connection with no body the client can read.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}

				slog.ErrorContext(r.Context(), "panic", "method", r.Method, "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
				writeError(w, New(http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", ""))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// NotFoundHandler answers any request no route matched. net/http's default is
// a plain-text 404 the client cannot read, so this keeps unknown routes inside
// the envelope too.
func NotFoundHandler() http.Handler {
	return Handle(func(http.ResponseWriter, *http.Request) error {
		return New(http.StatusNotFound, "NOT_FOUND", "Route not found")
	})
}

// WriteJSON writes v with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		slog.Error("encoding response", "error", err)
		http.Error(w, `{"error":"INTERNAL_SERVER_ERROR"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(bytes.TrimRight(buf.Bytes(), "\n"))
}

// NoContent writes a 204.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

const maxBodyBytes = 1 << 20

// ReadBody returns the raw request body, or nil when it is empty.
func ReadBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, New(http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Request body too large")
		}

		return nil, err
	}

	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil
	}

	return body, nil
}

// DecodeBody decodes a required JSON object into dst. A missing body is a
// validation failure on "body", matching Validate.Required in the C# service.
func DecodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	body, err := ReadBody(w, r)
	if err != nil {
		return err
	}

	if body == nil {
		return Invalid("body", "Request body is required")
	}

	return Decode(body, dst)
}

// Decode unmarshals body into dst, mapping JSON errors to 400s.
func Decode(body []byte, dst any) error {
	if err := json.Unmarshal(body, dst); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return Invalid(typeErr.Field, "'"+typeErr.Field+"' has the wrong type")
		}

		return Invalid("body", "Request body must be valid JSON")
	}

	return nil
}

// PathID reads a positive integer path parameter.
//
// The C# routes used {id:int} constraints, so a non-numeric segment never
// matched and fell through to a 404; a numeric but non-positive one reached
// the handler and failed validation. Both behaviours are kept.
func PathID(r *http.Request, name string) (int, error) {
	n, err := strconv.Atoi(r.PathValue(name))
	if err != nil {
		return 0, New(http.StatusNotFound, "NOT_FOUND", "Route not found")
	}

	if n <= 0 {
		return 0, Invalid(name, name+" must be a positive integer")
	}

	return n, nil
}
