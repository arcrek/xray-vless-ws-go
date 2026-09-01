package logserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestAuthTokenLifecycle(t *testing.T) {
	secret := generateAuthSecret()
	if len(secret) != 32 {
		t.Fatalf("expected 32 secret bytes, got %d", len(secret))
	}

	token := createSessionToken(secret)
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	if !validateSessionToken(token, secret) {
		t.Fatal("expected valid token to pass verification")
	}

	// Tampered token
	tampered := token + "a"
	if validateSessionToken(tampered, secret) {
		t.Fatal("expected tampered token to fail")
	}

	// Different secret
	diffSecret := generateAuthSecret()
	if validateSessionToken(token, diffSecret) {
		t.Fatal("expected token signed with diff secret to fail")
	}

	// Invalid format
	if validateSessionToken("invalid-format", secret) {
		t.Fatal("expected malformed token to fail")
	}
	if validateSessionToken("abc.1234", secret) {
		t.Fatal("expected invalid timestamp token to fail")
	}

	// Expired token simulation
	oldPayload := "1000000" // Year 1970
	oldToken := oldPayload + ".0123456789abcdef"
	if validateSessionToken(oldToken, secret) {
		t.Fatal("expected old expired timestamp to fail")
	}

	// Future token simulation
	futurePayload := "9999999999"
	futureToken := futurePayload + ".0123456789abcdef"
	if validateSessionToken(futureToken, secret) {
		t.Fatal("expected far-future timestamp to fail")
	}
}

func TestAuthTokenExpiry(t *testing.T) {
	secret := generateAuthSecret()
	// Token signed with timestamp 8 days ago
	oldTs := time.Now().Add(-8 * 24 * time.Hour).Unix()
	oldPayload := fmt.Sprintf("%d", oldTs)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(oldPayload))
	oldToken := oldPayload + "." + hex.EncodeToString(mac.Sum(nil))

	if validateSessionToken(oldToken, secret) {
		t.Fatal("expected 8-day old token to fail validation (expired)")
	}
}
