package logserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookieName = "xray_session"
	sessionDuration   = 7 * 24 * time.Hour
)

// generateAuthSecret generates 32 random bytes for session HMAC signing.
func generateAuthSecret() []byte {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		// Fallback to timestamp hash if crypto/rand fails
		h := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		return h[:]
	}
	return secret
}

// createSessionToken generates an HMAC-signed timestamp token: "<unix_sec>.<hmac_hex>"
func createSessionToken(secret []byte) string {
	now := time.Now().Unix()
	payload := strconv.FormatInt(now, 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

// validateSessionToken validates token signature and checks if token is within sessionDuration.
func validateSessionToken(token string, secret []byte) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	tsStr, sigHex := parts[0], parts[1]
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return false
	}

	// Check expiration
	tokenTime := time.Unix(ts, 0)
	if time.Since(tokenTime) > sessionDuration || time.Until(tokenTime) > 10*time.Minute {
		return false
	}

	// Verify HMAC
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(tsStr))
	expectedSig := mac.Sum(nil)
	actualSig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	return subtle.ConstantTimeCompare(expectedSig, actualSig) == 1
}

// setSessionCookie writes the xray_session cookie on response.
func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie removes the xray_session cookie.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
