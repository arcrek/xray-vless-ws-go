package logserver

import (
	"sync"
	"time"
)

// LogLine is one entry in the ring buffer, matching the JSON shape
// logging_site.py's /logs handler returns (id/time/type/text).
type LogLine struct {
	ID   int    `json:"id"`
	Time string `json:"time"`
	Type string `json:"type"` // "source" in Python's push_log(text, log_type) - XRAY/CLOUDFLARE/INFO/etc.
	Text string `json:"text"`
}

// Ring is a bounded, mutex-protected log buffer — the direct, boring port
// of Python's threading.Lock-guarded list (logging_site.py:63-93). No need
// for anything fancier at this log volume (a few lines/sec at most).
type Ring struct {
	mu      sync.Mutex
	maxLogs int
	logs    []LogLine
	seq     int
}

// NewRing creates a ring buffer capped at maxLogs entries.
func NewRing(maxLogs int) *Ring {
	if maxLogs <= 0 {
		maxLogs = 500 // match logging_site.py's default max_logs=500
	}
	return &Ring{maxLogs: maxLogs}
}

// Push appends one log line, dropping the oldest entry if the buffer is at
// capacity — ported from push_log() (logging_site.py:82-93).
func (r *Ring) Push(source, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	r.logs = append(r.logs, LogLine{
		ID:   r.seq,
		Time: time.Now().Format("15:04:05"),
		Type: source,
		Text: text,
	})
	if len(r.logs) > r.maxLogs {
		r.logs = r.logs[1:]
	}
}

// Since returns every log line with ID > lastID, plus the current sequence
// number - matching /logs's {new_logs, last_id} response shape
// (logging_site.py:127-138).
func (r *Ring) Since(lastID int) (newLogs []LogLine, currentSeq int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// logs are ID-ordered and IDs are monotonic, so a linear scan from the
	// end (the common case: lastID is recent) is fine at this volume; find
	// the first entry with ID > lastID.
	start := len(r.logs)
	for i, l := range r.logs {
		if l.ID > lastID {
			start = i
			break
		}
	}
	out := make([]LogLine, len(r.logs)-start)
	copy(out, r.logs[start:])
	return out, r.seq
}
