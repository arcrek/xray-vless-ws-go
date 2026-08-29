package logserver

import (
	"sync"
	"time"
)

const maxHistorySamples = 150 // 150 * 2s ticker = 5 minutes

// livenessFailureThreshold is the number of consecutive failed liveness
// probes (Phase 4) required before RecordLiveness flips xrayUp false — one
// transient dropped probe alone must not flash the Status panel red.
const livenessFailureThreshold = 2

// ThroughputSample is one point on the throughput sparkline.
type ThroughputSample struct {
	T       int64 `json:"t"` // unix seconds
	UpBps   int64 `json:"up_bps"`
	DownBps int64 `json:"down_bps"`
}

// StatsSnapshot is the /stats JSON response shape.
type StatsSnapshot struct {
	XrayUp           bool               `json:"xray_up"` // "started, not yet stopped" until Phase 4's liveness probe starts reporting — see RecordLiveness
	TunnelReady      bool               `json:"tunnel_ready"`
	ReadyConnections int                `json:"ready_connections"`
	Hostname         string             `json:"hostname"`
	UptimeSec        int64              `json:"uptime_sec"` // dashboard-process uptime (since NewStatusStore), NOT "time xray/tunnel have been serving"
	UplinkBps        int64              `json:"uplink_bps"`
	DownlinkBps      int64              `json:"downlink_bps"`
	UplinkTotal      int64              `json:"uplink_total"`
	DownlinkTotal    int64              `json:"downlink_total"`
	History          []ThroughputSample `json:"history"`
}

// StatusStore holds the latest system status + a bounded throughput history,
// mirroring Ring's mutex-protected-slice shape (ring.go) for logs.
type StatusStore struct {
	mu sync.Mutex

	startedAt        time.Time
	xrayUp           bool
	tunnelReady      bool
	readyConnections int
	hostname         string

	lastSampleAt   time.Time
	lastUpTotal    int64
	lastDownTotal  int64
	haveLastSample bool

	lastUpBps, lastDownBps int64
	history                []ThroughputSample

	consecutiveProbeFailures int // Phase 4's RecordLiveness debounce state
}

// NewStatusStore constructs a StatusStore whose uptime clock starts now.
func NewStatusStore() *StatusStore {
	return &StatusStore{startedAt: time.Now()}
}

// SetXrayUp records "xray-core started successfully / has been shut down".
// This is a boot/shutdown flag, not an ongoing liveness signal — once Phase
// 4's probe ticker starts, RecordLiveness supersedes this as xrayUp's
// ongoing source of truth (SetXrayUp is still used for the initial "just
// started" value and the shutdown reset).
func (s *StatusStore) SetXrayUp(up bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.xrayUp = up
}

// RecordLiveness applies a one-success-clears / N-consecutive-failures-flips
// debounce to xrayUp, so one transient failed probe doesn't flash the
// Status panel red. See Phase 4.
func (s *StatusStore) RecordLiveness(ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ok {
		s.consecutiveProbeFailures = 0
		s.xrayUp = true
		return
	}
	s.consecutiveProbeFailures++
	if s.consecutiveProbeFailures >= livenessFailureThreshold {
		s.xrayUp = false
	}
}

// SetTunnelStatus records the latest cloudflared /ready poll outcome.
func (s *StatusStore) SetTunnelStatus(ready bool, readyConnections int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tunnelReady = ready
	s.readyConnections = readyConnections
}

// SetHostname records the current tunnel hostname.
func (s *StatusStore) SetHostname(hostname string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hostname = hostname
}

// RecordTraffic diffs the given cumulative totals against the previous call
// to derive bytes/sec, and appends one sample to the bounded history. The
// first-ever call has no previous sample to diff against, so it records 0
// bps rather than a meaningless (total-0)/interval spike.
func (s *StatusStore) RecordTraffic(upTotal, downTotal int64) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	var upBps, downBps int64
	if s.haveLastSample {
		interval := now.Sub(s.lastSampleAt).Seconds()
		if interval > 0 {
			upBps = int64(float64(upTotal-s.lastUpTotal) / interval)
			downBps = int64(float64(downTotal-s.lastDownTotal) / interval)
		}
	}
	s.haveLastSample = true
	s.lastSampleAt = now
	s.lastUpTotal, s.lastDownTotal = upTotal, downTotal
	s.lastUpBps, s.lastDownBps = upBps, downBps

	s.history = append(s.history, ThroughputSample{T: now.Unix(), UpBps: upBps, DownBps: downBps})
	if len(s.history) > maxHistorySamples {
		s.history = s.history[1:]
	}
}

// Snapshot returns the current status + a copy of the history slice (so the
// caller's JSON encoding can't race a concurrent append).
func (s *StatusStore) Snapshot() StatsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	hist := make([]ThroughputSample, len(s.history))
	copy(hist, s.history)

	return StatsSnapshot{
		XrayUp:           s.xrayUp,
		TunnelReady:      s.tunnelReady,
		ReadyConnections: s.readyConnections,
		Hostname:         s.hostname,
		UptimeSec:        int64(time.Since(s.startedAt).Seconds()),
		UplinkBps:        s.lastUpBps,
		DownlinkBps:      s.lastDownBps,
		UplinkTotal:      s.lastUpTotal,
		DownlinkTotal:    s.lastDownTotal,
		History:          hist,
	}
}
