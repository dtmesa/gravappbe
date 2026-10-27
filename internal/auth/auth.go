// Package auth holds password hashing, JWT issuing and verification, and the
// one-time codes used by email confirmation and password reset.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Cost matches bcrypt.hash(password, 12) in the original Node backend and the
// C# port, so every stored hash stays verifiable. Changing it would require
// rehashing; do that opportunistically on login if it ever matters.
const Cost = 12

// dummyHash is a valid cost-12 hash of a random string nobody knows. Login
// compares against it when the username does not exist, so a missing user
// costs the same ~400ms of bcrypt as a wrong password and response time no
// longer reveals which usernames exist. It is a constant rather than computed
// at startup because generating it would add the same ~400ms to cold start.
const dummyHash = "$2a$12$AhyVyNNMK5llOaaajdushuor6AC8NzGRYSw32zIQybpDEI/lWhCzC"

// HashPassword hashes at Cost.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), Cost)
	return string(h), err
}

// CheckPassword reports whether password matches hash. golang.org/x/crypto's
// bcrypt verifies the $2a$ hashes BCrypt.Net produced.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// BurnPasswordCheck spends the same time as a real CheckPassword, for the
// path where there is no user to check against.
func BurnPasswordCheck(password string) {
	_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
}

// Claims is the token payload. userId and tokenVersion are JSON numbers, as
// jsonwebtoken and the C# service both emitted them.
type Claims struct {
	UserID       int `json:"userId"`
	TokenVersion int `json:"tokenVersion"`
	jwt.RegisteredClaims
}

// TokenTTL matches the 7-day expiry of the original tokens.
const TokenTTL = 7 * 24 * time.Hour

// Signer issues and verifies HS256 tokens.
type Signer struct {
	key []byte
	now func() time.Time
}

func NewSigner(secret string, now func() time.Time) *Signer {
	return &Signer{key: []byte(secret), now: now}
}

func (s *Signer) Sign(userID, tokenVersion int) (string, error) {
	claims := Claims{
		UserID:       userID,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(s.now().Add(TokenTTL)),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.key)
}

// Verify checks the signature and expiry (with zero leeway, as the C# service
// had ClockSkew = TimeSpan.Zero) and returns the claims.
func (s *Signer) Verify(token string) (Claims, error) {
	var claims Claims

	_, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) { return s.key, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil {
		return Claims{}, err
	}

	if claims.UserID <= 0 {
		return Claims{}, errors.New("auth: token has no userId")
	}

	return claims, nil
}

// BearerToken extracts the token from an Authorization header value.
func BearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}

	return strings.TrimSpace(token), true
}

// NewCode returns a cryptographically random six-digit code, zero-padded.
func NewCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%06d", n.Int64()), nil
}

// HashCode hashes a one-time code as unsalted uppercase hex SHA-256, matching
// Convert.ToHexString in the C# service. SHA-256 rather than bcrypt is
// deliberate: these are single-use, short-lived, low-entropy codes checked on
// every attempt, so bcrypt's slowness would add latency without security. The
// real defences are the per-row attempt cap and rate limiting.
func HashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// CodeMatches compares a submitted code against a stored hash in constant time.
func CodeMatches(storedHash, code string) bool {
	return subtle.ConstantTimeCompare([]byte(storedHash), []byte(HashCode(code))) == 1
}
