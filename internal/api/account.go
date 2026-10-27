package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/dtmesa/gravappbe/internal/auth"
	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/mail"
	"github.com/dtmesa/gravappbe/internal/model"
	"github.com/dtmesa/gravappbe/internal/store"
	"github.com/dtmesa/gravappbe/internal/valid"
)

const (
	// maxCodeAttempts wrong guesses before a code is burned.
	maxCodeAttempts = 5
	codeTTL         = 15 * time.Minute
)

var (
	errInvalidPassword = httpx.New(http.StatusUnauthorized, "INVALID_PASSWORD", "Incorrect password")
	errInvalidReset    = httpx.New(http.StatusBadRequest, "INVALID_RESET_CODE", "Invalid or expired code")
	errNoPendingEmail  = httpx.New(http.StatusNotFound, "NO_PENDING_EMAIL", "No pending email confirmation")
)

// decode reads and validates a required JSON body.
func decode[T valid.Validator](w http.ResponseWriter, r *http.Request) (T, error) {
	var v T

	if err := httpx.DecodeBody(w, r, &v); err != nil {
		return v, err
	}

	return v, v.Validate()
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) error {
	req, err := decode[valid.Register](w, r)
	if err != nil {
		return err
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}

	u, err := s.Store.CreateUser(r.Context(), req.Username, hash)
	if err != nil {
		return err
	}

	// The token is additive: today's app ignores it and logs in separately.
	token, err := s.Signer.Sign(u.ID, u.TokenVersion)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusCreated, model.RegisterResponse{ID: u.ID, Username: u.Username, Token: token})

	return nil
}

// login returns INVALID_PASSWORD for both an unknown username and a wrong
// password, so the response never says which usernames exist. The C# service
// returned INVALID_CREDENTIALS, a code the app does not recognise, so a failed
// login showed a generic "Login failed"; INVALID_PASSWORD is the code its login
// screen maps to "Invalid username or password".
//
// An unknown user still pays for a bcrypt compare, so response time does not
// reveal it either.
func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	req, err := decode[valid.Login](w, r)
	if err != nil {
		return err
	}

	u, err := s.Store.UserByUsername(r.Context(), req.Username)
	if err != nil {
		return err
	}

	if u == nil {
		auth.BurnPasswordCheck(req.Password)
		return errInvalidPassword
	}

	if !auth.CheckPassword(u.Password, req.Password) {
		return errInvalidPassword
	}

	return s.writeToken(w, u.ID, u.TokenVersion)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, caller model.User) error {
	u, err := s.Store.RequireUser(r.Context(), caller.ID)
	if err != nil {
		return err
	}

	// pendingEmail is derived purely from a live confirmation row, so TTL
	// expiry silently clears it with nothing else to clean up.
	pending, err := s.Store.ConfirmationCodes().Get(r.Context(), store.ConfirmationKey(u.ID))
	if err != nil {
		return err
	}

	resp := model.MeResponse{Username: u.Username, Email: u.Email, EmailConfirmed: u.EmailConfirmed}
	if pending != nil {
		resp.PendingEmail = &pending.Email
	}

	httpx.WriteJSON(w, http.StatusOK, resp)

	return nil
}

func (s *Server) updateUsername(w http.ResponseWriter, r *http.Request, u model.User) error {
	req, err := decode[valid.UpdateUsername](w, r)
	if err != nil {
		return err
	}

	if !auth.CheckPassword(u.Password, req.Password) {
		return errInvalidPassword
	}

	if err := s.Store.RenameUser(r.Context(), u.ID, u.Username, req.NewUsername); err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, model.UsernameResponse{Username: req.NewUsername})

	return nil
}

func (s *Server) updatePassword(w http.ResponseWriter, r *http.Request, u model.User) error {
	req, err := decode[valid.UpdatePassword](w, r)
	if err != nil {
		return err
	}

	if !auth.CheckPassword(u.Password, req.CurrentPassword) {
		return errInvalidPassword
	}

	if req.CurrentPassword == req.NewPassword {
		return httpx.New(http.StatusBadRequest, "SAME_PASSWORD", "New password must differ from current")
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		return err
	}

	version, ok, err := s.Store.UpdatePassword(r.Context(), u.ID, hash)
	if err != nil {
		return err
	}

	if !ok {
		return httpx.NotFound("User", "USER_NOT_FOUND")
	}

	// The version bump invalidates the caller's own token too, so issue a
	// fresh one or this session would fail its very next request.
	return s.writeToken(w, u.ID, version)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request, u model.User) error {
	req, err := decode[valid.DeleteAccount](w, r)
	if err != nil {
		return err
	}

	if !auth.CheckPassword(u.Password, req.Password) {
		return errInvalidPassword
	}

	// The user row and its claims go first, parent before children, so the
	// account stops working immediately.
	if err := s.Store.DeleteUser(r.Context(), u); err != nil {
		return err
	}

	if err := s.Store.DeleteUserData(r.Context(), u.ID); err != nil {
		return err
	}

	httpx.NoContent(w)

	return nil
}

// Recovery. Every response that depends on whether an email has an account is
// identical either way; these endpoints must never leak that signal.

func (s *Server) forgotUsername(w http.ResponseWriter, r *http.Request) error {
	req, err := decode[valid.EmailOnly](w, r)
	if err != nil {
		return err
	}

	// A second check keyed by the target email, on top of the per-IP one, so
	// an attacker rotating IPs cannot spam one victim's inbox.
	if err := s.Limiter.Ensure(r.Context(), s.limits.ForgotUsername, "email:"+store.NormalizeEmail(req.Email)); err != nil {
		return err
	}

	u, err := s.Store.UserByEmail(r.Context(), req.Email)
	if err != nil {
		return err
	}

	if u != nil {
		s.sendQuietly(r, req.Email, "Your Gravity username", mail.ForgotUsername(u.Username))
	}

	httpx.WriteJSON(w, http.StatusOK, model.MessageResponse{Message: "If that email is registered, we've sent the username to it."})

	return nil
}

func (s *Server) requestPasswordReset(w http.ResponseWriter, r *http.Request) error {
	req, err := decode[valid.EmailOnly](w, r)
	if err != nil {
		return err
	}

	if err := s.Limiter.Ensure(r.Context(), s.limits.PasswordResetRequest, "email:"+store.NormalizeEmail(req.Email)); err != nil {
		return err
	}

	u, err := s.Store.UserByEmail(r.Context(), req.Email)
	if err != nil {
		return err
	}

	if u != nil {
		code, err := auth.NewCode()
		if err != nil {
			return err
		}

		// Send before persisting: a failed send must never leave a usable
		// code in the database that nobody received.
		if s.sendQuietly(r, req.Email, "Your Gravity password reset code", mail.PasswordReset(code)) {
			if err := s.Store.ResetCodes().Put(r.Context(), u.ID, req.Email, auth.HashCode(code), codeTTL); err != nil {
				return err
			}
		}
	}

	httpx.WriteJSON(w, http.StatusOK, model.MessageResponse{Message: "If that email is registered, we've sent a reset code to it."})

	return nil
}

func (s *Server) verifyPasswordReset(w http.ResponseWriter, r *http.Request) error {
	req, err := decode[valid.VerifyPasswordReset](w, r)
	if err != nil {
		return err
	}

	codes := s.Store.ResetCodes()
	key := store.ResetKey(req.Email)

	code, err := codes.Get(r.Context(), key)
	if err != nil {
		return err
	}

	if code == nil {
		return errInvalidReset
	}

	// Too many attempts gets the same response as a wrong code, forcing a
	// fresh request rather than leaking "you were close".
	if code.Attempts >= maxCodeAttempts {
		if err := codes.Delete(r.Context(), key); err != nil {
			return err
		}

		return errInvalidReset
	}

	if !auth.CodeMatches(code.CodeHash, req.Code) {
		if err := codes.IncrementAttempts(r.Context(), key); err != nil {
			return err
		}

		return errInvalidReset
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		return err
	}

	version, ok, err := s.Store.UpdatePassword(r.Context(), code.UserID, hash)
	if err != nil {
		return err
	}

	if err := codes.Delete(r.Context(), key); err != nil {
		return err
	}

	// The account was deleted after the code was issued.
	if !ok {
		return errInvalidReset
	}

	// A fresh token keeps the device completing the reset logged in, while
	// the version bump invalidates every other outstanding token.
	return s.writeToken(w, code.UserID, version)
}

// sendQuietly sends an email for an unauthenticated recovery flow and reports
// whether it went out. A failure is logged, never returned: the C# service
// surfaced SES errors as a 5xx for a registered email where an unregistered
// one got 200, which revealed which emails have accounts.
func (s *Server) sendQuietly(r *http.Request, to, subject string, body mail.Body) bool {
	if err := s.Mail.Send(r.Context(), to, subject, body); err != nil {
		slog.ErrorContext(r.Context(), "recovery email failed", "subject", subject, "error", err)
		return false
	}

	return true
}

// Email management (authenticated).

func (s *Server) addEmail(w http.ResponseWriter, r *http.Request, u model.User) error {
	req, err := decode[valid.AddEmail](w, r)
	if err != nil {
		return err
	}

	// Keyed by the target email: it may not be the caller's own inbox.
	if err := s.Limiter.Ensure(r.Context(), s.limits.EmailMutate, "email:"+store.NormalizeEmail(req.Email)); err != nil {
		return err
	}

	if !auth.CheckPassword(u.Password, req.Password) {
		return errInvalidPassword
	}

	if u.Email != nil {
		return httpx.New(http.StatusConflict, "EMAIL_ALREADY_SET", "An email is already set on this account")
	}

	if err := s.sendConfirmation(r, u.ID, req.Email); err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusAccepted, model.PendingEmailResponse{PendingEmail: req.Email})

	return nil
}

func (s *Server) changeEmail(w http.ResponseWriter, r *http.Request, u model.User) error {
	req, err := decode[valid.ChangeEmail](w, r)
	if err != nil {
		return err
	}

	if err := s.Limiter.Ensure(r.Context(), s.limits.EmailMutate, "email:"+store.NormalizeEmail(req.NewEmail)); err != nil {
		return err
	}

	if !auth.CheckPassword(u.Password, req.Password) {
		return errInvalidPassword
	}

	if u.Email == nil {
		return httpx.New(http.StatusConflict, "NO_EMAIL_SET", "No email is set on this account yet")
	}

	// Overwrites any prior pending row, so switching targets mid-flow just
	// works; the current confirmed email stays valid for recovery until the
	// new one is confirmed.
	if err := s.sendConfirmation(r, u.ID, req.NewEmail); err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusAccepted, model.PendingEmailResponse{PendingEmail: req.NewEmail})

	return nil
}

func (s *Server) resendEmail(w http.ResponseWriter, r *http.Request, u model.User) error {
	pending, err := s.Store.ConfirmationCodes().Get(r.Context(), store.ConfirmationKey(u.ID))
	if err != nil {
		return err
	}

	if pending == nil {
		return errNoPendingEmail
	}

	// A new code rather than the same one, so an intercepted old code stops
	// working. If the send fails, the previous code and row are untouched.
	if err := s.sendConfirmation(r, u.ID, pending.Email); err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, model.PendingEmailResponse{PendingEmail: pending.Email})

	return nil
}

// confirmEmail returns 404 NO_PENDING_EMAIL where the C# service returned 400
// for the same code (resend already used 404). The client reads only the code.
func (s *Server) confirmEmail(w http.ResponseWriter, r *http.Request, u model.User) error {
	req, err := decode[valid.ConfirmEmail](w, r)
	if err != nil {
		return err
	}

	codes := s.Store.ConfirmationCodes()
	key := store.ConfirmationKey(u.ID)

	pending, err := codes.Get(r.Context(), key)
	if err != nil {
		return err
	}

	if pending == nil {
		return errNoPendingEmail
	}

	if pending.Attempts >= maxCodeAttempts {
		if err := codes.Delete(r.Context(), key); err != nil {
			return err
		}

		return errNoPendingEmail
	}

	if !auth.CodeMatches(pending.CodeHash, req.Code) {
		if err := codes.IncrementAttempts(r.Context(), key); err != nil {
			return err
		}

		return httpx.New(http.StatusBadRequest, "INVALID_CODE", "Invalid code")
	}

	fresh, err := s.Store.RequireUser(r.Context(), u.ID)
	if err != nil {
		return err
	}

	// Re-confirming the address already on the account: the claim transaction
	// would put and delete the same key, which DynamoDB rejects with a 500.
	// The email is already confirmed, so just clear the pending row.
	if fresh.Email != nil && store.NormalizeEmail(*fresh.Email) == pending.Email {
		if err := codes.Delete(r.Context(), key); err != nil {
			return err
		}
	} else if err := s.Store.ConfirmEmail(r.Context(), u.ID, fresh.Email, pending.Email); err != nil {
		// May be 409 EMAIL_TAKEN. Reaching it needs the right code, so it is
		// not an enumeration leak.
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, model.EmailConfirmedResponse{Email: pending.Email, EmailConfirmed: true})

	return nil
}

// sendConfirmation sends a new code, then stores it -- send before persist,
// so an SES rejection (an unverified recipient in sandbox mode) leaves no
// pending row the app would show a code-entry screen for. Unlike recovery, a
// failure is surfaced: this is the caller's own account, so there is nothing
// to leak.
func (s *Server) sendConfirmation(r *http.Request, userID int, email string) error {
	code, err := auth.NewCode()
	if err != nil {
		return err
	}

	if err := s.Mail.Send(r.Context(), email, "Confirm your Gravity email", mail.EmailConfirmation(code)); err != nil {
		return err
	}

	return s.Store.ConfirmationCodes().Put(r.Context(), userID, email, auth.HashCode(code), codeTTL)
}

func (s *Server) writeToken(w http.ResponseWriter, userID, version int) error {
	token, err := s.Signer.Sign(userID, version)
	if err != nil {
		return err
	}

	httpx.WriteJSON(w, http.StatusOK, model.TokenResponse{Token: token})

	return nil
}
