package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

func TestEnvelopeShapes(t *testing.T) {
	cases := []struct {
		name       string
		handler    http.Handler
		wantStatus int
		wantBody   string
	}{
		{
			"app error is a bare code",
			Handle(func(http.ResponseWriter, *http.Request) error { return NotFound("Workout", "WORKOUT_NOT_FOUND") }),
			404, `{"error":"WORKOUT_NOT_FOUND"}`,
		},
		{
			"wrapped app error still renders",
			Handle(func(http.ResponseWriter, *http.Request) error {
				return errors.Join(errors.New("context"), New(409, "USERNAME_TAKEN", ""))
			}),
			409, `{"error":"USERNAME_TAKEN"}`,
		},
		{
			"validation carries issues",
			Handle(func(http.ResponseWriter, *http.Request) error { return Invalid("name", "bad") }),
			400, `{"error":"VALIDATION_ERROR","issues":[{"path":"name","message":"bad"}]}`,
		},
		{
			"unknown error is an opaque 500",
			Handle(func(http.ResponseWriter, *http.Request) error { return errors.New("dynamo exploded") }),
			500, `{"error":"INTERNAL_SERVER_ERROR"}`,
		},
		{
			"panic is a 500 envelope",
			Recover(Handle(func(http.ResponseWriter, *http.Request) error { panic("boom") })),
			500, `{"error":"INTERNAL_SERVER_ERROR"}`,
		},
		{
			"unknown route is JSON, not text",
			NotFoundHandler(),
			404, `{"error":"NOT_FOUND"}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := serve(c.handler, "GET", "/", "")

			if rec.Code != c.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, c.wantStatus)
			}

			if got := rec.Body.String(); got != c.wantBody {
				t.Errorf("body = %s, want %s", got, c.wantBody)
			}

			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("content-type = %q", ct)
			}
		})
	}
}

func TestRetryAfterHeader(t *testing.T) {
	rec := serve(Handle(func(http.ResponseWriter, *http.Request) error { return TooManyRequests(42) }), "GET", "/", "")

	if rec.Code != 429 || rec.Header().Get("Retry-After") != "42" || rec.Body.String() != `{"error":"RATE_LIMITED"}` {
		t.Errorf("got %d %q %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body)
	}
}

func TestDecodeBody(t *testing.T) {
	type req struct {
		Name string `json:"name"`
	}

	h := func(body string) *httptest.ResponseRecorder {
		return serve(Handle(func(w http.ResponseWriter, r *http.Request) error {
			var v req
			if err := DecodeBody(w, r, &v); err != nil {
				return err
			}

			WriteJSON(w, 200, v)

			return nil
		}), "POST", "/", body)
	}

	if rec := h(""); rec.Code != 400 || !strings.Contains(rec.Body.String(), `"path":"body"`) {
		t.Errorf("empty body: %d %s", rec.Code, rec.Body)
	}

	if rec := h("{not json"); rec.Code != 400 {
		t.Errorf("bad json: %d %s", rec.Code, rec.Body)
	}

	if rec := h(`{"name":5}`); rec.Code != 400 || !strings.Contains(rec.Body.String(), `"path":"name"`) {
		t.Errorf("wrong type: %d %s", rec.Code, rec.Body)
	}

	// Go matches JSON keys case-insensitively, as the C# service did.
	if rec := h(`{"NAME":"x"}`); rec.Code != 200 || rec.Body.String() != `{"name":"x"}` {
		t.Errorf("case-insensitive: %d %s", rec.Code, rec.Body)
	}
}

func TestPathID(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /w/{id}", Handle(func(w http.ResponseWriter, r *http.Request) error {
		id, err := PathID(r, "id")
		if err != nil {
			return err
		}

		WriteJSON(w, 200, id)

		return nil
	}))

	for target, want := range map[string]int{"/w/7": 200, "/w/abc": 404, "/w/0": 400, "/w/-3": 400} {
		if rec := serve(mux, "GET", target, ""); rec.Code != want {
			t.Errorf("%s = %d, want %d", target, rec.Code, want)
		}
	}
}
