// Package api is the HTTP surface: routing, middleware and one handler per
// endpoint. Handlers return errors (see httpx.Handle) instead of writing
// failure responses themselves.
package api

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dtmesa/gravappbe/internal/auth"
	"github.com/dtmesa/gravappbe/internal/config"
	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/mail"
	"github.com/dtmesa/gravappbe/internal/model"
	"github.com/dtmesa/gravappbe/internal/ratelimit"
	"github.com/dtmesa/gravappbe/internal/store"
)

// Deps is everything the handlers need, built once in main. It is the whole of
// the dependency-injection story.
type Deps struct {
	Config  config.Config
	Store   *store.Store
	Signer  *auth.Signer
	Limiter *ratelimit.Limiter
	Mail    mail.Sender
	Now     func() time.Time
}

type Server struct {
	Deps
	limits config.RateLimits
}

// New returns the complete handler, middleware included.
func New(d Deps) http.Handler {
	s := &Server{Deps: d, limits: d.Config.RateLimits}

	mux := http.NewServeMux()
	s.routes(mux)

	var h http.Handler = mux
	h = s.generalRateLimit(h)
	h = s.cors(h)
	h = s.deadline(h)
	h = trimTrailingSlash(h)

	return httpx.Recover(h)
}

// authedFunc is a handler that runs only for a valid, current token.
type authedFunc func(w http.ResponseWriter, r *http.Request, u model.User) error

// public wraps an unauthenticated handler, applying policy per client IP
// first when given.
func (s *Server) public(policy *config.Policy, h httpx.HandlerFunc) http.Handler {
	return httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		if policy != nil {
			if err := s.Limiter.Ensure(r.Context(), *policy, "ip:"+clientIP(r)); err != nil {
				return err
			}
		}

		return h(w, r)
	})
}

// private wraps an authenticated handler. A per-route policy is keyed by user
// rather than IP: mobile clients behind carrier NAT share addresses, so IP
// keys let one person lock out strangers, while a rotating attacker evades
// them.
func (s *Server) private(policy *config.Policy, h authedFunc) http.Handler {
	return httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		u, err := s.authenticate(r)
		if err != nil {
			return err
		}

		if policy != nil {
			if err := s.Limiter.Ensure(r.Context(), *policy, "user:"+strconv.Itoa(u.ID)); err != nil {
				return err
			}
		}

		return h(w, r, u)
	})
}

// authenticate verifies the bearer token, then checks its tokenVersion against
// the stored user -- how a password change or reset invalidates every other
// session.
//
// The user is read eventually-consistently first, which halves the cost of the
// lookup every authenticated request makes. A miss or a version mismatch is
// retried with a consistent read before rejecting, so a token issued moments
// ago (register then login, or a password change) is never wrongly refused --
// the client logs out on any failed /auth/me, so a false 401 would be visible.
func (s *Server) authenticate(r *http.Request) (model.User, error) {
	token, ok := auth.BearerToken(r.Header.Get("Authorization"))
	if !ok {
		return model.User{}, httpx.Unauthorized()
	}

	claims, err := s.Signer.Verify(token)
	if err != nil {
		return model.User{}, httpx.Unauthorized()
	}

	u, err := s.Store.User(r.Context(), claims.UserID, false)
	if err != nil {
		return model.User{}, err
	}

	if u == nil || u.TokenVersion != claims.TokenVersion {
		if u, err = s.Store.User(r.Context(), claims.UserID, true); err != nil {
			return model.User{}, err
		}
	}

	if u == nil || u.TokenVersion != claims.TokenVersion {
		return model.User{}, httpx.Unauthorized()
	}

	return *u, nil
}

// generalRateLimit is the blanket per-IP volume cap. Off by default in Lambda,
// where API Gateway stage throttling does the same job without a DynamoDB
// write in front of every request.
func (s *Server) generalRateLimit(next http.Handler) http.Handler {
	if !s.limits.Enabled || !s.limits.GeneralEnabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.Limiter.Ensure(r.Context(), s.limits.General, "ip:"+clientIP(r)); err != nil {
			httpx.RenderError(w, r, err)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// cors pins the single configured origin in production and reflects any
// origin otherwise (the Express backend's allowedOrigins = [true]). It matters
// only for a browser client; native app traffic sends no Origin.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		allowed := origin != "" && (!s.Config.Production || origin == s.Config.ClientOrigin)

		if allowed {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")

				if req := r.Header.Get("Access-Control-Request-Headers"); req != "" {
					h.Set("Access-Control-Allow-Headers", req)
				}

				w.WriteHeader(http.StatusNoContent)

				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// deadline bounds the request so a slow dependency fails as a clean 5xx with
// an envelope, rather than the Lambda timing out with no response. Inside
// Lambda the context already carries the invocation deadline; leave a margin
// before it.
func (s *Server) deadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := time.Now().Add(s.Config.RequestTimeout)

		if d, ok := r.Context().Deadline(); ok && d.Add(-500*time.Millisecond).Before(limit) {
			limit = d.Add(-500 * time.Millisecond)
		}

		ctx, cancel := context.WithDeadline(r.Context(), limit)
		defer cancel()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// trimTrailingSlash makes "/workouts/" route like "/workouts", as ASP.NET
// routing did.
func trimTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; len(p) > 1 && strings.HasSuffix(p, "/") {
			r2 := r.Clone(r.Context())
			r2.URL.Path = strings.TrimRight(p, "/")

			if r2.URL.Path == "" {
				r2.URL.Path = "/"
			}

			r2.URL.RawPath = ""
			r = r2
		}

		next.ServeHTTP(w, r)
	})
}

// clientIP is the caller's address. The Lambda adapter sets RemoteAddr to API
// Gateway's sourceIp (no port); the local server's carries a port.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}

	if r.RemoteAddr == "" {
		return "unknown"
	}

	return r.RemoteAddr
}
