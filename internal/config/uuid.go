package config

import (
	"crypto/rand"
	"fmt"
)

// newUUIDv4 generates a random RFC 4122 version-4 UUID without pulling in an
// extra dependency — this is the only place a fresh UUID is needed (first-run
// default XRAY_UUID), stdlib crypto/rand is sufficient.
func newUUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is effectively unrecoverable on any real
		// platform; fall back to a fixed-looking but still valid-shaped
		// UUID rather than panicking, so config loading never crashes here.
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
