// Package testutil runs the real API over DynamoDB Local for integration
// tests. Each test binary gets its own table prefix, so runs never see each
// other's data, and tables are dropped when it exits. Without
// DYNAMODB_ENDPOINT set, integration tests skip and only unit tests run.
package testutil

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dtmesa/gravappbe/internal/api"
	"github.com/dtmesa/gravappbe/internal/auth"
	"github.com/dtmesa/gravappbe/internal/config"
	"github.com/dtmesa/gravappbe/internal/dynamo"
	"github.com/dtmesa/gravappbe/internal/mail"
	"github.com/dtmesa/gravappbe/internal/ratelimit"
	"github.com/dtmesa/gravappbe/internal/store"
)

var shared *dynamo.DB

// Main wraps a package's TestMain: it creates uniquely prefixed tables when
// DynamoDB Local is configured, runs the tests, and drops the tables.
func Main(m *testing.M) {
	code := func() int {
		endpoint := os.Getenv("DYNAMODB_ENDPOINT")
		if endpoint == "" {
			return m.Run()
		}

		ctx := context.Background()

		db, err := dynamo.Connect(ctx, "us-west-2", endpoint, "T"+Random(6)+"_")
		if err != nil {
			fmt.Fprintln(os.Stderr, "connect:", err)
			return 1
		}

		if err := db.EnsureTables(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "create tables:", err)
			return 1
		}

		defer func() { _ = db.DropTables(ctx) }()

		shared = db

		return m.Run()
	}()

	os.Exit(code)
}

// Random returns n random lowercase hex characters.
func Random(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)[:n]
}

// Clock is a settable clock. Tests share wall-clock-free time so timestamps,
// windows and expiry are deterministic.
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.t
}

func (c *Clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.t = t
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.t = c.t.Add(d)
}

// Env is one running API.
type Env struct {
	T      *testing.T
	Server *httptest.Server
	Store  *store.Store
	DB     *dynamo.DB
	Clock  *Clock
	Mail   *mail.Recorder
	Config config.Config
}

// Option adjusts the configuration before the server starts.
type Option func(*config.Config)

// WithRateLimits enables limiting with the given policies. Policy names are
// made unique per call so tests never share counters.
func WithRateLimits(limits config.RateLimits) Option {
	return func(c *config.Config) {
		suffix := "_" + Random(6)

		for _, p := range []*config.Policy{
			&limits.General, &limits.Login, &limits.Register, &limits.AuthMutate, &limits.ForgotUsername,
			&limits.PasswordResetRequest, &limits.PasswordResetVerify, &limits.EmailMutate,
		} {
			p.Name += suffix
			if p.MaxRequests == 0 {
				p.MaxRequests, p.WindowSeconds = 1000, 60
			}
		}

		limits.Enabled = true
		c.RateLimits = limits
	}
}

// Start runs a fresh server over the shared tables, skipping the test when
// DynamoDB Local is not configured.
func Start(t *testing.T, opts ...Option) *Env {
	t.Helper()

	if shared == nil {
		t.Skip("DYNAMODB_ENDPOINT not set; skipping integration test")
	}

	cfg := config.Config{
		JWTSecret:      "test-secret",
		Region:         "us-west-2",
		TablePrefix:    "",
		EmailSender:    "console",
		RequestTimeout: 20 * time.Second,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	// A fixed instant with a zero-ending millisecond, so any response that
	// trims trailing zeros (".25Z") fails loudly.
	//
	// It must stay in the future. DynamoDB Local runs a real TTL sweeper
	// against wall-clock time, so a rate-limit counter or one-time code whose
	// expiresAt (computed from this clock) is already in the past gets deleted
	// mid-test -- which showed up as lost rate-limit increments and vanishing
	// confirmation codes.
	clock := &Clock{t: time.Date(2031, 6, 15, 12, 0, 0, 250_000_000, time.UTC)}
	recorder := &mail.Recorder{}

	st := store.New(shared, clock.Now)

	handler := api.New(api.Deps{
		Config:  cfg,
		Store:   st,
		Signer:  auth.NewSigner(cfg.JWTSecret, clock.Now),
		Limiter: ratelimit.New(shared, clock.Now, cfg.RateLimits.Enabled),
		Mail:    recorder,
		Now:     clock.Now,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Env{T: t, Server: srv, Store: st, DB: shared, Clock: clock, Mail: recorder, Config: cfg}
}

// Response is a captured HTTP response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// JSON decodes the body into v, failing the test on error.
func (r Response) JSON(t *testing.T, v any) {
	t.Helper()

	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("decode %s: %v", r.Body, err)
	}
}

// Code returns the "error" field of an error envelope.
func (r Response) Code() string {
	var body struct {
		Error string `json:"error"`
	}

	_ = json.Unmarshal(r.Body, &body)

	return body.Error
}

func (r Response) String() string {
	return fmt.Sprintf("%d %s", r.Status, r.Body)
}

// Client sends requests as one user (or anonymously, with no token).
type Client struct {
	env   *Env
	Token string
}

func (e *Env) Anon() *Client { return &Client{env: e} }

// Do sends a request. body may be nil, a string (sent verbatim), or any value
// (sent as JSON).
func (c *Client) Do(method, path string, body any) Response {
	c.env.T.Helper()

	var reader io.Reader

	switch b := body.(type) {
	case nil:
	case string:
		reader = bytes.NewBufferString(b)
	default:
		data, err := json.Marshal(b)
		if err != nil {
			c.env.T.Fatal(err)
		}

		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.env.Server.URL+path, reader)
	if err != nil {
		c.env.T.Fatal(err)
	}

	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	res, err := c.env.Server.Client().Do(req)
	if err != nil {
		c.env.T.Fatal(err)
	}

	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		c.env.T.Fatal(err)
	}

	return Response{Status: res.StatusCode, Header: res.Header, Body: data}
}

// Expect sends a request and fails the test unless the status matches.
func (c *Client) Expect(status int, method, path string, body any) Response {
	c.env.T.Helper()

	res := c.Do(method, path, body)
	if res.Status != status {
		c.env.T.Fatalf("%s %s: got %s, want %d", method, path, res, status)
	}

	return res
}

// User registers a fresh user and returns a client logged in as them.
func (e *Env) User() (*Client, string) {
	e.T.Helper()

	username := "u" + Random(10)
	return e.UserNamed(username, "password1"), username
}

func (e *Env) UserNamed(username, password string) *Client {
	e.T.Helper()

	anon := e.Anon()
	anon.Expect(http.StatusCreated, "POST", "/auth/register", map[string]string{"username": username, "password": password})

	var tok struct {
		Token string `json:"token"`
	}

	anon.Expect(http.StatusOK, "POST", "/auth/login", map[string]string{"username": username, "password": password}).JSON(e.T, &tok)

	return &Client{env: e, Token: tok.Token}
}
