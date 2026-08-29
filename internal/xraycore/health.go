package xraycore

import (
	"context"
	"net"
	"time"
)

// ProbeListener reports whether a TCP listener is currently accepting
// connections at addr. This is a raw connect-and-close, not a VLESS
// handshake — enough to detect "the process is hung and the listener died"
// without the complexity of a protocol-aware health check (see plan
// Phase 4 Key Insights for why that tradeoff was made).
func ProbeListener(ctx context.Context, addr string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
