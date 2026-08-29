package logserver

import "testing"

func TestRingPushAndSince(t *testing.T) {
	r := NewRing(500)
	r.Push("XRAY", "line 1")
	r.Push("CLOUDFLARE", "line 2")

	logs, seq := r.Since(0)
	if len(logs) != 2 {
		t.Fatalf("got %d logs, want 2", len(logs))
	}
	if seq != 2 {
		t.Errorf("seq = %d, want 2", seq)
	}
	if logs[0].Type != "XRAY" || logs[0].Text != "line 1" {
		t.Errorf("logs[0] = %+v, want Type=XRAY Text='line 1'", logs[0])
	}

	logs2, _ := r.Since(1)
	if len(logs2) != 1 || logs2[0].Text != "line 2" {
		t.Errorf("Since(1) = %+v, want just line 2", logs2)
	}

	logs3, _ := r.Since(2)
	if len(logs3) != 0 {
		t.Errorf("Since(2) = %+v, want empty", logs3)
	}
}

func TestRingCapsAtMaxLogsDropOldest(t *testing.T) {
	r := NewRing(3)
	r.Push("A", "1")
	r.Push("A", "2")
	r.Push("A", "3")
	r.Push("A", "4") // should evict "1"

	logs, _ := r.Since(0)
	if len(logs) != 3 {
		t.Fatalf("got %d logs, want 3 (capped)", len(logs))
	}
	if logs[0].Text != "2" {
		t.Errorf("oldest surviving log = %q, want %q (log '1' should have been dropped)", logs[0].Text, "2")
	}
}

func TestRingDefaultMaxLogs(t *testing.T) {
	r := NewRing(0)
	if r.maxLogs != 500 {
		t.Errorf("maxLogs = %d, want default 500", r.maxLogs)
	}
}
