package logserver

import (
	"testing"
	"time"
)

func TestRecordTrafficFirstSampleIsZeroBps(t *testing.T) {
	s := NewStatusStore()
	s.RecordTraffic(1000, 2000)

	snap := s.Snapshot()
	if snap.UplinkBps != 0 || snap.DownlinkBps != 0 {
		t.Errorf("first sample bps = (%d, %d), want (0, 0) — no previous total to diff against", snap.UplinkBps, snap.DownlinkBps)
	}
	if snap.UplinkTotal != 1000 || snap.DownlinkTotal != 2000 {
		t.Errorf("first sample totals = (%d, %d), want (1000, 2000)", snap.UplinkTotal, snap.DownlinkTotal)
	}
	if len(snap.History) != 1 {
		t.Fatalf("expected 1 history sample, got %d", len(snap.History))
	}
	if snap.History[0].UpBps != 0 || snap.History[0].DownBps != 0 {
		t.Errorf("first history sample = %+v, want 0 bps both directions", snap.History[0])
	}
}

func TestRecordTrafficComputesRateOnSecondSample(t *testing.T) {
	s := NewStatusStore()

	// Fake the first sample's timestamp directly so the second call's
	// interval is deterministic rather than depending on real elapsed time
	// between two RecordTraffic calls in a fast test.
	s.mu.Lock()
	s.haveLastSample = true
	s.lastSampleAt = time.Now().Add(-2 * time.Second)
	s.lastUpTotal = 1000
	s.lastDownTotal = 2000
	s.mu.Unlock()

	s.RecordTraffic(3000, 2200) // +2000 up, +200 down over ~2s

	snap := s.Snapshot()
	if snap.UplinkBps < 900 || snap.UplinkBps > 1100 {
		t.Errorf("uplink_bps = %d, want ~1000 (2000 bytes / 2s)", snap.UplinkBps)
	}
	if snap.DownlinkBps < 90 || snap.DownlinkBps > 110 {
		t.Errorf("downlink_bps = %d, want ~100 (200 bytes / 2s)", snap.DownlinkBps)
	}
	if snap.UplinkTotal != 3000 || snap.DownlinkTotal != 2200 {
		t.Errorf("totals = (%d, %d), want (3000, 2200)", snap.UplinkTotal, snap.DownlinkTotal)
	}
}

func TestRecordTrafficHistoryCapsAtMaxSamples(t *testing.T) {
	s := NewStatusStore()
	for i := 0; i < maxHistorySamples+50; i++ {
		s.RecordTraffic(int64(i), int64(i))
	}

	snap := s.Snapshot()
	if len(snap.History) != maxHistorySamples {
		t.Fatalf("history length = %d, want capped at %d", len(snap.History), maxHistorySamples)
	}
}

func TestSnapshotHistoryIsACopyNotAnAlias(t *testing.T) {
	s := NewStatusStore()
	s.RecordTraffic(100, 100)

	snap := s.Snapshot()
	snap.History[0].UpBps = 999999 // mutate the returned slice

	snap2 := s.Snapshot()
	if snap2.History[0].UpBps == 999999 {
		t.Fatal("mutating a returned Snapshot's History leaked back into the store — Snapshot must copy, not alias")
	}
}

func TestSetXrayUpRoundTrips(t *testing.T) {
	s := NewStatusStore()
	if s.Snapshot().XrayUp {
		t.Fatal("XrayUp should start false")
	}

	s.SetXrayUp(true)
	if !s.Snapshot().XrayUp {
		t.Fatal("SetXrayUp(true) did not round-trip through Snapshot().XrayUp")
	}

	s.SetXrayUp(false)
	if s.Snapshot().XrayUp {
		t.Fatal("SetXrayUp(false) (shutdown reset) did not round-trip through Snapshot().XrayUp")
	}
}

func TestSetTunnelStatusAndSetHostname(t *testing.T) {
	s := NewStatusStore()
	s.SetTunnelStatus(true, 2)
	s.SetHostname("foo.trycloudflare.com")

	snap := s.Snapshot()
	if !snap.TunnelReady || snap.ReadyConnections != 2 {
		t.Errorf("tunnel status = (%v, %d), want (true, 2)", snap.TunnelReady, snap.ReadyConnections)
	}
	if snap.Hostname != "foo.trycloudflare.com" {
		t.Errorf("hostname = %q, want foo.trycloudflare.com", snap.Hostname)
	}
}

func TestRecordLivenessDebounce(t *testing.T) {
	s := NewStatusStore()
	s.SetXrayUp(true)

	// One failure alone must not flip xrayUp.
	s.RecordLiveness(false)
	if !s.Snapshot().XrayUp {
		t.Fatal("a single failed probe flipped xray_up false, want it to require 2 consecutive failures")
	}

	// A second consecutive failure flips it.
	s.RecordLiveness(false)
	if s.Snapshot().XrayUp {
		t.Fatal("2 consecutive failed probes did not flip xray_up false")
	}

	// One success immediately clears it, regardless of prior failure count.
	s.RecordLiveness(true)
	if !s.Snapshot().XrayUp {
		t.Fatal("a single successful probe after failures did not immediately flip xray_up back true")
	}

	// The failure counter must have reset too — one more single failure
	// alone should again not flip it.
	s.RecordLiveness(false)
	if !s.Snapshot().XrayUp {
		t.Fatal("failure counter did not reset after a success — one failure alone flipped xray_up false")
	}
}
