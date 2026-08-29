package xraycore

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestProbeListenerSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("starting test listener: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	if !ProbeListener(context.Background(), ln.Addr().String(), time.Second) {
		t.Error("ProbeListener returned false against a live listener, want true")
	}
}

func TestProbeListenerFailureAfterClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("starting test listener: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	if ProbeListener(context.Background(), addr, time.Second) {
		t.Error("ProbeListener returned true against a closed listener, want false")
	}
}

func TestProbeListenerRespectsTimeout(t *testing.T) {
	// 10.255.255.1 is a non-routable, non-local address commonly used in
	// tests to force a connection attempt that hangs (rather than an
	// immediate refusal) — bounds the test's own runtime via a short
	// timeout rather than the production 2s default.
	start := time.Now()
	ok := ProbeListener(context.Background(), "10.255.255.1:65535", 200*time.Millisecond)
	elapsed := time.Since(start)

	if ok {
		t.Error("ProbeListener returned true against an unroutable address, want false")
	}
	if elapsed > 2*time.Second {
		t.Errorf("ProbeListener took %s to respect a 200ms timeout", elapsed)
	}
}
