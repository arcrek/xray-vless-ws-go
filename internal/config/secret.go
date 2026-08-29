package config

import (
	"crypto/rand"
	"encoding/hex"
)

// newSecretToken generates a random hex-encoded token n bytes long, used as
// the WORKER_PASSWORD fallback when it's missing from .env — same
// crypto/rand idiom as newUUIDv4 (uuid.go), just a different shape (this
// value is a URL query parameter, not a UUID).
func newSecretToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is effectively unrecoverable on any real
		// platform; fall back to a fixed-looking placeholder rather than
		// panicking, so config loading never crashes here (same reasoning
		// as newUUIDv4's fallback).
		return "0000000000000000000000000000000000000000000000"
	}
	return hex.EncodeToString(b)
}
