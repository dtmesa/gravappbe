package config

import (
	"strings"
	"testing"
)

func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()

	// Clear everything Load reads, so the developer's own environment can't
	// leak into the test.
	for _, name := range []string{
		"JWT_SECRET", "NODE_ENV", "CLIENT_ORIGIN", "AWS_REGION", "AWS_DEFAULT_REGION", "DYNAMODB_ENDPOINT",
		"TABLE_PREFIX", "EMAIL_SENDER", "EMAIL_FROM_ADDRESS", "AWS_LAMBDA_FUNCTION_NAME", "RATE_LIMIT_ENABLED",
		"RATE_LIMIT_GENERAL_ENABLED", "RATE_LIMIT_LOGIN_MAX", "RATE_LIMIT_LOGIN_WINDOW_SECONDS",
	} {
		t.Setenv(name, "")
	}

	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, map[string]string{"JWT_SECRET": "s"})

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if c.TablePrefix != "Gravity_" || c.Region != "us-west-2" || c.EmailSender != "console" || c.Production {
		t.Errorf("defaults = %+v", c)
	}

	if !c.RateLimits.Enabled || !c.RateLimits.GeneralEnabled {
		t.Error("rate limiting should default on, including the general cap, outside Lambda")
	}

	if c.RateLimits.Login.MaxRequests != 5 || c.RateLimits.Login.WindowSeconds != 900 {
		t.Errorf("login policy = %+v", c.RateLimits.Login)
	}
}

func TestLoadFailures(t *testing.T) {
	cases := map[string]struct {
		vars map[string]string
		want string
	}{
		"missing secret":         {map[string]string{}, "JWT_SECRET"},
		"production no origin":   {map[string]string{"JWT_SECRET": "s", "NODE_ENV": "production", "EMAIL_FROM_ADDRESS": "a@b"}, "CLIENT_ORIGIN"},
		"ses without from":       {map[string]string{"JWT_SECRET": "s", "EMAIL_SENDER": "ses"}, "EMAIL_FROM_ADDRESS"},
		"unknown sender":         {map[string]string{"JWT_SECRET": "s", "EMAIL_SENDER": "pigeon"}, "EMAIL_SENDER"},
		"lambda defaults to ses": {map[string]string{"JWT_SECRET": "s", "AWS_LAMBDA_FUNCTION_NAME": "fn"}, "EMAIL_FROM_ADDRESS"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			setEnv(t, c.vars)

			if _, err := Load(); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want mention of %s", err, c.want)
			}
		})
	}
}

// Inside Lambda the console sender (which logs codes) is never chosen by
// omission, and the general per-IP limiter is off in favour of API Gateway
// throttling.
func TestLambdaDefaults(t *testing.T) {
	setEnv(t, map[string]string{"JWT_SECRET": "s", "AWS_LAMBDA_FUNCTION_NAME": "fn", "EMAIL_FROM_ADDRESS": "a@b"})

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if c.EmailSender != "ses" || c.RateLimits.GeneralEnabled {
		t.Errorf("lambda config = sender %q, general %v", c.EmailSender, c.RateLimits.GeneralEnabled)
	}
}

func TestPolicyOverrides(t *testing.T) {
	setEnv(t, map[string]string{"JWT_SECRET": "s", "RATE_LIMIT_LOGIN_MAX": "50", "RATE_LIMIT_LOGIN_WINDOW_SECONDS": "60"})

	c, _ := Load()
	if c.RateLimits.Login.MaxRequests != 50 || c.RateLimits.Login.WindowSeconds != 60 {
		t.Errorf("login = %+v", c.RateLimits.Login)
	}
}
