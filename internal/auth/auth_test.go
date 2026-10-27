package auth

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPasswordRoundTripAndCost(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(hash, "$2a$12$") {
		t.Errorf("hash %q is not $2a$ at cost 12", hash)
	}

	if !CheckPassword(hash, "correct horse") || CheckPassword(hash, "wrong") {
		t.Error("CheckPassword mismatch")
	}
}

func TestDummyHashIsValidCost12(t *testing.T) {
	if !strings.HasPrefix(dummyHash, "$2a$12$") || len(dummyHash) != 60 {
		t.Fatalf("dummy hash malformed: %q", dummyHash)
	}

	start := time.Now()
	BurnPasswordCheck("anything")

	// A malformed hash fails instantly; a real one takes real time.
	if time.Since(start) < 20*time.Millisecond {
		t.Error("BurnPasswordCheck returned too fast to be doing a cost-12 compare")
	}
}

func TestTokenClaimsAreNumbers(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := NewSigner("secret", func() time.Time { return now })

	token, err := s.Sign(42, 3)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatal(err)
	}

	if raw["userId"] != float64(42) || raw["tokenVersion"] != float64(3) {
		t.Errorf("claims not numeric: %s", payload)
	}

	if raw["exp"] != float64(now.Add(TokenTTL).Unix()) {
		t.Errorf("exp = %v", raw["exp"])
	}

	claims, err := s.Verify(token)
	if err != nil || claims.UserID != 42 || claims.TokenVersion != 3 {
		t.Errorf("Verify = %+v, %v", claims, err)
	}
}

func TestVerifyRejects(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := NewSigner("secret", func() time.Time { return now })

	good, _ := s.Sign(1, 0)

	expired, _ := NewSigner("secret", func() time.Time { return now.Add(-TokenTTL - time.Second) }).Sign(1, 0)
	otherKey, _ := NewSigner("other", func() time.Time { return now }).Sign(1, 0)

	if _, err := s.Verify(good); err != nil {
		t.Fatalf("good token rejected: %v", err)
	}

	for name, token := range map[string]string{
		"expired":   expired,
		"other key": otherKey,
		"garbage":   "a.b.c",
		// alg=none must never be accepted.
		"none": base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." +
			base64.RawURLEncoding.EncodeToString([]byte(`{"userId":1,"tokenVersion":0,"exp":9999999999}`)) + ".",
	} {
		if _, err := s.Verify(token); err == nil {
			t.Errorf("%s token accepted", name)
		}
	}
}

func TestBearerToken(t *testing.T) {
	for header, want := range map[string]string{
		"Bearer abc":  "abc",
		"bearer abc":  "abc",
		"Bearer  abc": "abc",
		"Basic abc":   "",
		"Bearer":      "",
		"":            "",
	} {
		got, ok := BearerToken(header)
		if got != want || ok != (want != "") {
			t.Errorf("BearerToken(%q) = %q, %v", header, got, ok)
		}
	}
}

func TestCodes(t *testing.T) {
	six := regexp.MustCompile(`^\d{6}$`)

	for range 200 {
		code, err := NewCode()
		if err != nil || !six.MatchString(code) {
			t.Fatalf("NewCode = %q, %v", code, err)
		}
	}

	// Uppercase hex SHA-256, matching Convert.ToHexString(SHA256(...)).
	const want = "8D969EEF6ECAD3C29A3A629280E686CF0C3F5D5A86AFF3CA12020C923ADC6C92"
	if got := HashCode("123456"); got != want {
		t.Errorf("HashCode = %s", got)
	}

	if !CodeMatches(want, "123456") || CodeMatches(want, "123457") {
		t.Error("CodeMatches mismatch")
	}
}
