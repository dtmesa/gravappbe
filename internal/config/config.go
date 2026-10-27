// Package config reads the environment once, at startup, into a plain struct.
// Nothing below main reads os.Getenv directly.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is every setting the service reads. Variable names are unchanged from
// the C# service so template.yaml and existing .env files keep working.
type Config struct {
	JWTSecret string

	// Production pins CORS to ClientOrigin. NODE_ENV is a leftover name from the
	// Express backend, kept for template compatibility.
	Production   bool
	ClientOrigin string

	Region         string
	DynamoEndpoint string // set only for DynamoDB Local
	TablePrefix    string

	EmailSender string // "console" or "ses"
	EmailFrom   string

	RateLimits RateLimits

	// RequestTimeout bounds every request so a slow dependency fails as a clean
	// 5xx with a body rather than a Lambda timeout with none.
	RequestTimeout time.Duration
}

// Policy is a fixed-window rate limit.
type Policy struct {
	Name          string
	MaxRequests   int
	WindowSeconds int
}

// RateLimits mirrors RateLimitOptions.cs. Auth endpoints are much stricter than
// the general cap: General guards against generic volume, the rest against
// credential stuffing and inbox spam.
type RateLimits struct {
	Enabled bool

	// General is the blanket per-IP cap on every request. It costs a DynamoDB
	// write in front of every request, so in Lambda it is off by default and
	// API Gateway stage throttling does the job at the edge instead.
	GeneralEnabled bool
	General        Policy

	Login                Policy
	Register             Policy
	AuthMutate           Policy
	ForgotUsername       Policy
	PasswordResetRequest Policy
	PasswordResetVerify  Policy
	EmailMutate          Policy
}

// InLambda reports whether the process is running inside AWS Lambda.
func InLambda() bool {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
}

// Load reads the environment and fails on anything the service cannot start
// without.
func Load() (Config, error) {
	c := Config{
		JWTSecret:      os.Getenv("JWT_SECRET"),
		Production:     os.Getenv("NODE_ENV") == "production",
		ClientOrigin:   strings.TrimSpace(os.Getenv("CLIENT_ORIGIN")),
		Region:         firstNonEmpty(os.Getenv("AWS_REGION"), os.Getenv("AWS_DEFAULT_REGION"), "us-west-2"),
		DynamoEndpoint: strings.TrimSpace(os.Getenv("DYNAMODB_ENDPOINT")),
		TablePrefix:    firstNonEmpty(os.Getenv("TABLE_PREFIX"), "Gravity_"),
		EmailFrom:      os.Getenv("EMAIL_FROM_ADDRESS"),
		RequestTimeout: time.Duration(intEnv("REQUEST_TIMEOUT_SECONDS", 9)) * time.Second,
	}

	if c.JWTSecret == "" {
		return c, errors.New("JWT_SECRET is missing")
	}

	if c.Production && c.ClientOrigin == "" {
		return c, errors.New("CLIENT_ORIGIN must be set in production")
	}

	// SES is the hard default inside Lambda: the console sender logs OTP codes,
	// and selecting it by omission there would put them in CloudWatch.
	defaultSender := "console"
	if c.Production || InLambda() {
		defaultSender = "ses"
	}

	c.EmailSender = firstNonEmpty(os.Getenv("EMAIL_SENDER"), defaultSender)

	if c.EmailSender != "console" && c.EmailSender != "ses" {
		return c, errors.New(`EMAIL_SENDER must be "console" or "ses"`)
	}

	if c.EmailSender == "ses" && c.EmailFrom == "" {
		return c, errors.New("EMAIL_FROM_ADDRESS is required when EMAIL_SENDER=ses")
	}

	c.RateLimits = RateLimits{
		Enabled:        boolEnv("RATE_LIMIT_ENABLED", true),
		GeneralEnabled: boolEnv("RATE_LIMIT_GENERAL_ENABLED", !InLambda()),

		General:              policy("GENERAL", 300, 300),
		Login:                policy("LOGIN", 5, 900),
		Register:             policy("REGISTER", 5, 3600),
		AuthMutate:           policy("AUTH_MUTATE", 10, 900),
		ForgotUsername:       policy("FORGOT_USERNAME", 5, 3600),
		PasswordResetRequest: policy("PASSWORD_RESET_REQUEST", 5, 3600),
		PasswordResetVerify:  policy("PASSWORD_RESET_VERIFY", 10, 900),
		EmailMutate:          policy("EMAIL_MUTATE", 10, 900),
	}

	return c, nil
}

// policy reads RATE_LIMIT_<NAME>_MAX and RATE_LIMIT_<NAME>_WINDOW_SECONDS,
// falling back to the given defaults -- the same variables the C# service read.
func policy(name string, maxRequests, windowSeconds int) Policy {
	return Policy{
		Name:          name,
		MaxRequests:   intEnv("RATE_LIMIT_"+name+"_MAX", maxRequests),
		WindowSeconds: intEnv("RATE_LIMIT_"+name+"_WINDOW_SECONDS", windowSeconds),
	}
}

func intEnv(name string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}

	return fallback
}

func boolEnv(name string, fallback bool) bool {
	if v, err := strconv.ParseBool(os.Getenv(name)); err == nil {
		return v
	}

	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}

	return ""
}
