package api_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/dtmesa/gravappbe/internal/config"
	"github.com/dtmesa/gravappbe/internal/testutil"
)

type token struct {
	Token string `json:"token"`
}

func TestRegisterReturnsTokenAndClaimsUsername(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	anon := env.Anon()

	name := "u" + testutil.Random(8)

	var reg struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Token    string `json:"token"`
	}

	anon.Expect(201, "POST", "/auth/register", map[string]string{"username": name, "password": "password1"}).JSON(t, &reg)

	if reg.ID <= 0 || reg.Username != name || reg.Token == "" {
		t.Fatalf("register response = %+v", reg)
	}

	// The additive token works on its own, without a separate login.
	me := env.Anon()
	me.Token = reg.Token
	me.Expect(200, "GET", "/auth/me", nil)

	if res := anon.Do("POST", "/auth/register", map[string]string{"username": name, "password": "password1"}); res.Status != 409 || res.Code() != "USERNAME_TAKEN" {
		t.Errorf("duplicate register = %s", res)
	}
}

func TestRegisterValidation(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)

	res := env.Anon().Do("POST", "/auth/register", map[string]string{"username": "ab", "password": "short"})
	if res.Status != 400 || res.Code() != "VALIDATION_ERROR" || !strings.Contains(string(res.Body), `"issues"`) {
		t.Errorf("got %s", res)
	}

	if res := env.Anon().Do("POST", "/auth/register", nil); res.Status != 400 || res.Code() != "VALIDATION_ERROR" {
		t.Errorf("missing body = %s", res)
	}
}

func TestLogin(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	_, name := env.User()
	anon := env.Anon()

	anon.Expect(200, "POST", "/auth/login", map[string]string{"username": name, "password": "password1"})

	// Unknown user and wrong password are indistinguishable, and use the code
	// the app's login screen actually recognises.
	for _, body := range []map[string]string{
		{"username": name, "password": "wrong-password"},
		{"username": "nobody" + testutil.Random(6), "password": "password1"},
	} {
		if res := anon.Do("POST", "/auth/login", body); res.Status != 401 || res.Code() != "INVALID_PASSWORD" {
			t.Errorf("login %v = %s", body, res)
		}
	}
}

func TestUnauthenticatedRequestsGetTokenError(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)

	for _, tok := range []string{"", "garbage", "a.b.c"} {
		c := env.Anon()
		c.Token = tok

		if res := c.Do("GET", "/auth/me", nil); res.Status != 401 || string(res.Body) != `{"error":"TOKEN"}` {
			t.Errorf("token %q: %s", tok, res)
		}
	}
}

func TestMeHasExplicitNulls(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, name := env.User()

	res := c.Expect(200, "GET", "/auth/me", nil)

	want := `{"username":"` + name + `","email":null,"emailConfirmed":false,"pendingEmail":null}`
	if string(res.Body) != want {
		t.Errorf("me = %s, want %s", res.Body, want)
	}
}

func TestChangeUsername(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, old := env.User()
	next := "n" + testutil.Random(8)

	if res := c.Do("PATCH", "/auth/username", map[string]string{"newUsername": next, "password": "nope-nope"}); res.Code() != "INVALID_PASSWORD" {
		t.Errorf("wrong password = %s", res)
	}

	res := c.Expect(200, "PATCH", "/auth/username", map[string]string{"newUsername": next, "password": "password1"})
	if string(res.Body) != `{"username":"`+next+`"}` {
		t.Errorf("body = %s", res.Body)
	}

	// Renaming to the current name is a no-op, not a conflict with itself.
	c.Expect(200, "PATCH", "/auth/username", map[string]string{"newUsername": next, "password": "password1"})

	env.Anon().Expect(200, "POST", "/auth/login", map[string]string{"username": next, "password": "password1"})

	// The old name was released.
	env.Anon().Expect(201, "POST", "/auth/register", map[string]string{"username": old, "password": "password1"})

	// And a taken name conflicts.
	if res := c.Do("PATCH", "/auth/username", map[string]string{"newUsername": old, "password": "password1"}); res.Code() != "USERNAME_TAKEN" {
		t.Errorf("taken = %s", res)
	}
}

func TestChangePasswordRotatesTokens(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, name := env.User()
	oldToken := c.Token

	if res := c.Do("PATCH", "/auth/password", map[string]string{"currentPassword": "wrong-one", "newPassword": "password2"}); res.Code() != "INVALID_PASSWORD" {
		t.Errorf("wrong current = %s", res)
	}

	if res := c.Do("PATCH", "/auth/password", map[string]string{"currentPassword": "password1", "newPassword": "password1"}); res.Code() != "SAME_PASSWORD" {
		t.Errorf("same password = %s", res)
	}

	var fresh token
	c.Expect(200, "PATCH", "/auth/password", map[string]string{"currentPassword": "password1", "newPassword": "password2"}).JSON(t, &fresh)

	// The caller's session survives on the reissued token; every older token
	// is invalidated.
	c.Token = fresh.Token
	c.Expect(200, "GET", "/auth/me", nil)

	stale := env.Anon()
	stale.Token = oldToken

	if res := stale.Do("GET", "/auth/me", nil); res.Code() != "TOKEN" {
		t.Errorf("old token still works: %s", res)
	}

	env.Anon().Expect(200, "POST", "/auth/login", map[string]string{"username": name, "password": "password2"})
}

func TestDeleteAccountCascadesAndReleasesClaims(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, name := env.User()

	w := createWorkout(t, c, "Push")
	ex := createExercise(t, c, w, "Bench")
	ws := createSession(t, c, w)
	es := createExerciseSession(t, c, w, ws, ex)
	c.Expect(201, "POST", setsPath(w, ws, es), nil)

	if res := c.Do("DELETE", "/auth/delete", map[string]string{"password": "wrong-pass"}); res.Code() != "INVALID_PASSWORD" {
		t.Errorf("wrong password = %s", res)
	}

	// DELETE with a JSON body is part of the contract.
	c.Expect(204, "DELETE", "/auth/delete", map[string]string{"password": "password1"})

	if res := c.Do("GET", "/auth/me", nil); res.Code() != "TOKEN" {
		t.Errorf("deleted account's token still works: %s", res)
	}

	env.Anon().Expect(201, "POST", "/auth/register", map[string]string{"username": name, "password": "password1"})

	assertGone(t, env, w, ws, es)
}

func TestEmailAddConfirmAndChange(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	addr := "Person." + testutil.Random(6) + "@Example.com"
	lower := strings.ToLower(addr)

	if res := c.Do("POST", "/auth/email/resend", nil); res.Status != 404 || res.Code() != "NO_PENDING_EMAIL" {
		t.Errorf("resend with nothing pending = %s", res)
	}

	if res := c.Do("POST", "/auth/email", map[string]string{"email": addr, "password": "bad-password"}); res.Code() != "INVALID_PASSWORD" {
		t.Errorf("wrong password = %s", res)
	}

	// The 202 echoes the address as typed; it is stored normalized.
	res := c.Expect(202, "POST", "/auth/email", map[string]string{"email": addr, "password": "password1"})
	if string(res.Body) != `{"pendingEmail":"`+addr+`"}` {
		t.Errorf("add = %s", res.Body)
	}

	var me struct {
		Email        *string `json:"email"`
		Confirmed    bool    `json:"emailConfirmed"`
		PendingEmail *string `json:"pendingEmail"`
	}

	c.Expect(200, "GET", "/auth/me", nil).JSON(t, &me)

	if me.PendingEmail == nil || *me.PendingEmail != lower || me.Email != nil {
		t.Errorf("me after add = %+v", me)
	}

	if res := c.Do("POST", "/auth/email/confirm", map[string]string{"code": wrongCode(lastCode(t, env, addr))}); res.Status != 400 || res.Code() != "INVALID_CODE" {
		t.Errorf("wrong code = %s", res)
	}

	res = c.Expect(200, "POST", "/auth/email/confirm", map[string]string{"code": lastCode(t, env, addr)})
	if string(res.Body) != `{"email":"`+lower+`","emailConfirmed":true}` {
		t.Errorf("confirm = %s", res.Body)
	}

	c.Expect(200, "GET", "/auth/me", nil).JSON(t, &me)

	if me.Email == nil || *me.Email != lower || !me.Confirmed || me.PendingEmail != nil {
		t.Errorf("me after confirm = %+v", me)
	}

	if res := c.Do("POST", "/auth/email", map[string]string{"email": "x@y.z", "password": "password1"}); res.Code() != "EMAIL_ALREADY_SET" {
		t.Errorf("second add = %s", res)
	}

	if res := c.Do("POST", "/auth/email/confirm", map[string]string{"code": "123456"}); res.Status != 404 || res.Code() != "NO_PENDING_EMAIL" {
		t.Errorf("confirm with nothing pending = %s", res)
	}

	// Re-confirming the address already on the account used to 500 (the
	// transaction put and deleted the same key). It now succeeds.
	c.Expect(202, "PATCH", "/auth/email", map[string]string{"newEmail": addr, "password": "password1"})
	c.Expect(200, "POST", "/auth/email/confirm", map[string]string{"code": lastCode(t, env, addr)})

	// A real change moves the claim.
	next := "n" + testutil.Random(6) + "@example.com"
	c.Expect(202, "PATCH", "/auth/email", map[string]string{"newEmail": next, "password": "password1"})
	c.Expect(200, "POST", "/auth/email/resend", nil)
	c.Expect(200, "POST", "/auth/email/confirm", map[string]string{"code": lastCode(t, env, next)})

	// The old address is free again for another account.
	other, _ := env.User()
	other.Expect(202, "POST", "/auth/email", map[string]string{"email": addr, "password": "password1"})
	other.Expect(200, "POST", "/auth/email/confirm", map[string]string{"code": lastCode(t, env, addr)})
}

func TestEmailTakenAndAttemptCap(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	a, _ := env.User()
	b, _ := env.User()
	addr := "shared" + testutil.Random(6) + "@example.com"

	a.Expect(202, "POST", "/auth/email", map[string]string{"email": addr, "password": "password1"})
	a.Expect(200, "POST", "/auth/email/confirm", map[string]string{"code": lastCode(t, env, addr)})

	// A pending (unconfirmed) target never blocks anyone; the conflict only
	// surfaces at confirmation, after the code proves mailbox ownership.
	b.Expect(202, "POST", "/auth/email", map[string]string{"email": addr, "password": "password1"})

	if res := b.Do("POST", "/auth/email/confirm", map[string]string{"code": lastCode(t, env, addr)}); res.Status != 409 || res.Code() != "EMAIL_TAKEN" {
		t.Errorf("taken = %s", res)
	}

	// Five wrong guesses burn the code.
	other := "cap" + testutil.Random(6) + "@example.com"
	b.Expect(202, "POST", "/auth/email", map[string]string{"email": other, "password": "password1"})
	good := lastCode(t, env, other)

	for range 5 {
		b.Do("POST", "/auth/email/confirm", map[string]string{"code": wrongCode(good)})
	}

	if res := b.Do("POST", "/auth/email/confirm", map[string]string{"code": good}); res.Code() != "NO_PENDING_EMAIL" {
		t.Errorf("code survived the attempt cap: %s", res)
	}
}

func TestForgotUsernameDoesNotLeakRegistration(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, name := env.User()
	addr := "fu" + testutil.Random(6) + "@example.com"

	c.Expect(202, "POST", "/auth/email", map[string]string{"email": addr, "password": "password1"})
	c.Expect(200, "POST", "/auth/email/confirm", map[string]string{"code": lastCode(t, env, addr)})

	known := env.Anon().Expect(200, "POST", "/auth/forgot-username", map[string]string{"email": strings.ToUpper(addr)})

	msg, ok := env.Mail.Last(strings.ToUpper(addr))
	if !ok || !strings.Contains(msg.Body.Text, name) {
		t.Errorf("username email = %+v, %v", msg, ok)
	}

	unknownAddr := "nobody" + testutil.Random(6) + "@example.com"
	unknown := env.Anon().Expect(200, "POST", "/auth/forgot-username", map[string]string{"email": unknownAddr})

	if string(known.Body) != string(unknown.Body) {
		t.Errorf("responses differ: %s vs %s", known.Body, unknown.Body)
	}

	if env.Mail.Count(unknownAddr) != 0 {
		t.Error("emailed an unregistered address")
	}
}

func TestPasswordReset(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, name := env.User()
	addr := "pr" + testutil.Random(6) + "@example.com"

	c.Expect(202, "POST", "/auth/email", map[string]string{"email": addr, "password": "password1"})
	c.Expect(200, "POST", "/auth/email/confirm", map[string]string{"code": lastCode(t, env, addr)})

	anon := env.Anon()
	anon.Expect(200, "POST", "/auth/password-reset/request", map[string]string{"email": addr})
	code := lastCode(t, env, addr)

	verify := func(code string) testutil.Response {
		return anon.Do("POST", "/auth/password-reset/verify", map[string]string{"email": addr, "code": code, "newPassword": "password9"})
	}

	if res := verify(wrongCode(code)); res.Status != 400 || res.Code() != "INVALID_RESET_CODE" {
		t.Errorf("wrong code = %s", res)
	}

	var tok token
	res := verify(code)
	if res.Status != 200 {
		t.Fatalf("verify = %s", res)
	}

	res.JSON(t, &tok)

	// Every older token is invalidated; the reset device's token works.
	if res := c.Do("GET", "/auth/me", nil); res.Code() != "TOKEN" {
		t.Errorf("pre-reset token still works: %s", res)
	}

	fresh := env.Anon()
	fresh.Token = tok.Token
	fresh.Expect(200, "GET", "/auth/me", nil)

	anon.Expect(200, "POST", "/auth/login", map[string]string{"username": name, "password": "password9"})

	// The code is single-use.
	if res := verify(code); res.Code() != "INVALID_RESET_CODE" {
		t.Errorf("code reused: %s", res)
	}

	// Codes expire after 15 minutes even if DynamoDB's TTL has not deleted them.
	anon.Expect(200, "POST", "/auth/password-reset/request", map[string]string{"email": addr})
	late := lastCode(t, env, addr)
	env.Clock.Advance(16 * time.Minute)

	if res := verify(late); res.Code() != "INVALID_RESET_CODE" {
		t.Errorf("expired code accepted: %s", res)
	}
}

func TestPasswordResetAttemptCap(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	addr := "cap" + testutil.Random(6) + "@example.com"

	c.Expect(202, "POST", "/auth/email", map[string]string{"email": addr, "password": "password1"})
	c.Expect(200, "POST", "/auth/email/confirm", map[string]string{"code": lastCode(t, env, addr)})

	anon := env.Anon()
	anon.Expect(200, "POST", "/auth/password-reset/request", map[string]string{"email": addr})
	good := lastCode(t, env, addr)

	for range 5 {
		anon.Do("POST", "/auth/password-reset/verify", map[string]string{"email": addr, "code": wrongCode(good), "newPassword": "password9"})
	}

	if res := anon.Do("POST", "/auth/password-reset/verify", map[string]string{"email": addr, "code": good, "newPassword": "password9"}); res.Code() != "INVALID_RESET_CODE" {
		t.Errorf("code survived the attempt cap: %s", res)
	}
}

// A failed send for a registered email used to surface as a 5xx, while an
// unregistered email got 200 -- revealing which emails have accounts.
func TestRecoverySendFailureIsSilent(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t)
	c, _ := env.User()
	addr := "sf" + testutil.Random(6) + "@example.com"

	c.Expect(202, "POST", "/auth/email", map[string]string{"email": addr, "password": "password1"})
	c.Expect(200, "POST", "/auth/email/confirm", map[string]string{"code": lastCode(t, env, addr)})

	env.Mail.SetFail(errors.New("ses: sandbox rejected recipient"))

	env.Anon().Expect(200, "POST", "/auth/password-reset/request", map[string]string{"email": addr})
	env.Anon().Expect(200, "POST", "/auth/forgot-username", map[string]string{"email": addr})

	env.Mail.SetFail(nil)

	// No code was stored for the email that never went out.
	if res := env.Anon().Do("POST", "/auth/password-reset/verify", map[string]string{"email": addr, "code": "123456", "newPassword": "password9"}); res.Code() != "INVALID_RESET_CODE" {
		t.Errorf("verify = %s", res)
	}
}

func TestRateLimits(t *testing.T) {
	t.Parallel()
	env := testutil.Start(t, testutil.WithRateLimits(config.RateLimits{
		Login: config.Policy{Name: "LOGIN", MaxRequests: 2, WindowSeconds: 900},
	}))

	anon := env.Anon()
	body := map[string]string{"username": "nobody" + testutil.Random(6), "password": "password1"}

	for range 2 {
		if res := anon.Do("POST", "/auth/login", body); res.Code() != "INVALID_PASSWORD" {
			t.Fatalf("login within the limit = %s", res)
		}
	}

	res := anon.Do("POST", "/auth/login", body)
	if res.Status != 429 || string(res.Body) != `{"error":"RATE_LIMITED"}` || res.Header.Get("Retry-After") == "" {
		t.Errorf("third login = %s (Retry-After %q)", res, res.Header.Get("Retry-After"))

		out, _ := env.DB.Client.Scan(context.Background(), &dynamodb.ScanInput{TableName: aws.String(env.DB.T.RateLimits)})
		for _, item := range out.Items {
			if k := item["key"].(*types.AttributeValueMemberS).Value; strings.HasPrefix(k, env.Config.RateLimits.Login.Name+"#") {
				t.Logf("DIAG %s count=%v", k, item["count"])
			}
		}
	}

	// The window resets.
	env.Clock.Advance(16 * time.Minute)

	if res := anon.Do("POST", "/auth/login", body); res.Status == 429 {
		t.Error("still limited after the window")
	}

	// Authenticated policies are per user, so one user's limit does not
	// affect another on the same address.
	env = testutil.Start(t, testutil.WithRateLimits(config.RateLimits{
		AuthMutate: config.Policy{Name: "MUTATE", MaxRequests: 1, WindowSeconds: 900},
	}))

	a, _ := env.User()
	b, _ := env.User()

	a.Do("PATCH", "/auth/username", map[string]string{"newUsername": "x" + testutil.Random(8), "password": "password1"})

	if res := a.Do("PATCH", "/auth/username", map[string]string{"newUsername": "x" + testutil.Random(8), "password": "password1"}); res.Status != 429 {
		t.Errorf("a not limited: %s", res)
	}

	if res := b.Do("PATCH", "/auth/username", map[string]string{"newUsername": "x" + testutil.Random(8), "password": "password1"}); res.Status == 429 {
		t.Errorf("b limited by a's requests: %s", res)
	}
}

var sixDigits = regexp.MustCompile(`\b\d{6}\b`)

// lastCode reads the code from the most recent email to addr.
func lastCode(t *testing.T, env *testutil.Env, addr string) string {
	t.Helper()

	msg, ok := env.Mail.Last(addr)
	if !ok {
		t.Fatalf("no email sent to %s", addr)
	}

	code := sixDigits.FindString(msg.Body.Text)
	if code == "" {
		t.Fatalf("no code in %q", msg.Body.Text)
	}

	return code
}

func wrongCode(code string) string {
	if code == "000000" {
		return "000001"
	}

	return "000000"
}
