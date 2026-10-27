package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dtmesa/gravappbe/internal/config"
	"github.com/dtmesa/gravappbe/internal/testutil"
)

// Wire-level guarantees the app depends on, checked on raw bytes.

func TestRootStatus(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)

	if res := env.Anon().Expect(200, "GET", "/", nil); string(res.Body) != `{"status":"ok"}` {
		t.Errorf("root = %s", res.Body)
	}
}

// The app reads only response.data.error, so every failure -- including ones
// net/http would answer in plain text -- must be the JSON envelope.
func TestEveryFailureIsTheEnvelope(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()

	cases := []struct {
		name, method, path string
		body               any
		status             int
		code               string
	}{
		{"unknown route", "GET", "/nope", nil, 404, "NOT_FOUND"},
		{"wrong method on a known path", "PUT", "/auth/me", nil, 404, "NOT_FOUND"},
		{"malformed JSON", "POST", "/workouts", "{not json", 400, "VALIDATION_ERROR"},
		{"wrong JSON type", "POST", "/workouts", `{"name":5}`, 400, "VALIDATION_ERROR"},
		{"missing body", "POST", "/workouts", nil, 400, "VALIDATION_ERROR"},
		{"non-numeric id", "GET", "/workouts/abc", nil, 404, "NOT_FOUND"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := c.Do(tc.method, tc.path, tc.body)

			if res.Status != tc.status || res.Code() != tc.code {
				t.Errorf("got %s, want %d %s", res, tc.status, tc.code)
			}

			if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("content-type = %q", ct)
			}
		})
	}
}

// The fixed test clock sits on a zero-ending millisecond; Go's default time
// marshaling would print ".25Z".
func TestTimestampsKeepThreeFractionalDigits(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()

	res := c.Expect(201, "POST", "/workouts", map[string]string{"name": "Push"})
	if !strings.Contains(string(res.Body), `"createdAt":"2031-06-15T12:00:00.250Z"`) {
		t.Errorf("create = %s", res.Body)
	}

	// The create response and a later read agree byte for byte.
	w := createWorkout(t, c, "Pull")
	read := c.Expect(200, "GET", "/workouts", nil)

	if !strings.Contains(string(read.Body), w.CreatedAt.String()) || !strings.Contains(string(read.Body), ".250Z") {
		t.Errorf("list = %s", read.Body)
	}
}

func TestTrailingSlashRoutesLikeWithout(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()

	c.Expect(201, "POST", "/workouts/", map[string]string{"name": "Push"})
	c.Expect(200, "GET", "/workouts/", nil)
}

func TestCORS(t *testing.T) {
	t.Parallel()

	preflight := func(env *testutil.Env, origin string) *http.Response {
		req, _ := http.NewRequest("OPTIONS", env.Server.URL+"/workouts", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "PATCH")
		req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")

		res, err := env.Server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}

		res.Body.Close()

		return res
	}

	// Development reflects any origin.
	dev := testutil.Start(t)

	res := preflight(dev, "http://localhost:8081")
	if res.StatusCode != 204 || res.Header.Get("Access-Control-Allow-Origin") != "http://localhost:8081" ||
		!strings.Contains(res.Header.Get("Access-Control-Allow-Methods"), "PATCH") ||
		res.Header.Get("Access-Control-Allow-Headers") != "authorization,content-type" {
		t.Errorf("dev preflight = %d %v", res.StatusCode, res.Header)
	}

	// Production pins the configured origin.
	prod := testutil.Start(t, func(c *config.Config) {
		c.Production = true
		c.ClientOrigin = "https://app.example"
	})

	if res := preflight(prod, "https://app.example"); res.Header.Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Errorf("allowed origin rejected: %v", res.Header)
	}

	if res := preflight(prod, "https://evil.example"); res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("foreign origin allowed: %v", res.Header)
	}
}
