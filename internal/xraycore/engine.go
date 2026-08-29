package xraycore

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/xtls/xray-core/common/log"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/stats"
)

// statsEmail is the shared VLESS client "email" every inbound's client
// entry is tagged with (config.go), and the key xray-core's dispatcher uses
// to name the aggregate traffic counters below — see plan Decision Log #2.
const statsEmail = "default"

// LogSink receives one xray-core log line at a time. Using a plain callback
// (rather than a typed channel + LogLine struct) avoids Phase 5's logserver
// package needing to import xraycore's types just to consume them — the
// caller (main.go) closes over whatever tagging/buffering it wants.
type LogSink func(line string)

// logHandler adapts xray-core's global common/log.Handler interface to a
// LogSink callback. xray-core only supports one registered handler
// process-wide (log.RegisterHandler discards whatever was registered
// before), so Engine.New registers this once.
type logHandler struct {
	sink LogSink
}

func (h *logHandler) Handle(msg log.Message) {
	if h.sink == nil {
		return
	}
	// A panic inside the sink (e.g. a full unbuffered channel send with no
	// receiver) must not take down xray-core's internal logging goroutine.
	defer func() { _ = recover() }()
	h.sink(msg.String())
}

// Engine wraps an in-process xray-core instance so main.go never touches
// xray-core internals directly.
type Engine struct {
	instance    *core.Instance
	upCounter   stats.Counter // nil if the stats app never activated (see New)
	downCounter stats.Counter
	closeOnce   sync.Once
	closeErr    error
}

// New builds and starts an xray-core instance from the given JSON config
// (see BuildConfig). xray-core log lines are routed to sink if non-nil.
//
// core.StartInstance loads the config, constructs the instance, and starts
// it in one call. The config format name is "json" — main/json/json.go
// registers it as Name: "JSON", but core.RegisterConfigLoader lowercases
// the key before storing it (core/config.go), so the lookup at
// core.LoadConfig time is against "json"; verified against the pinned
// source, not assumed from documentation snippets (an earlier attempt at
// "JSON" here failed with "core: Unable to load config in JSON").
func New(cfgBytes []byte, sink LogSink) (engine *Engine, err error) {
	if sink != nil {
		log.RegisterHandler(&logHandler{sink: sink})
	}

	// xray-core running in-process means a panic inside its own code would
	// otherwise crash this whole binary. Recover here so a bad config or an
	// xray-core bug surfaces as an error instead of taking down
	// cloudflared/logserver/CI goroutines too.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("xraycore: panic starting instance: %v", r)
		}
	}()

	instance, startErr := core.StartInstance("json", cfgBytes)
	if startErr != nil {
		return nil, fmt.Errorf("xraycore: StartInstance: %w", startErr)
	}

	engine = &Engine{instance: instance}

	// Register (not just look up) both counters once, eagerly. This only
	// fails while the manager behind GetFeature is still stats.NoopManager
	// — i.e. the "stats" config block (config.go) never took effect — which
	// is the one real failure mode worth surfacing; GetFeature itself is
	// never nil (core.Instance always registers a NoopManager as a
	// fallback, verified against pinned source — see plan Phase 1 Key
	// Insights). Leaving both counters nil on failure makes Traffic()
	// report ok=false for the life of the process, correctly signalling
	// "stats never activated" instead of an ambiguous per-tick nil.
	if mgr, ok := instance.GetFeature(stats.ManagerType()).(stats.Manager); ok {
		up, upErr := mgr.GetOrRegisterCounter("user>>>" + statsEmail + ">>>traffic>>>uplink")
		down, downErr := mgr.GetOrRegisterCounter("user>>>" + statsEmail + ">>>traffic>>>downlink")
		if upErr == nil && downErr == nil {
			engine.upCounter, engine.downCounter = up, down
		}
	}

	return engine, nil
}

// Traffic returns cumulative uplink/downlink bytes across all VLESS ports
// (every client shares statsEmail, so xray-core's dispatcher already sums
// them into one counter pair — see plan Decision Log #2). ok=false only
// means "stats never activated at startup" — once true, it's always true
// for the life of the process, and 0,0 is a legitimate "no traffic yet"
// reading, not a failure.
func (e *Engine) Traffic() (uplink, downlink int64, ok bool) {
	if e == nil || e.upCounter == nil || e.downCounter == nil {
		return 0, 0, false
	}
	return e.upCounter.Value(), e.downCounter.Value(), true
}

// Start is a no-op today — core.StartInstance already starts the instance.
// Kept as a method (rather than folding entirely into New) so main.go's
// lifecycle shape (New → Start(ctx) → <-ctx.Done() → Close) stays uniform
// with Phase 3's Supervisor, and so a future xray-core version that
// separates "loaded" from "started" doesn't require an API change here.
func (e *Engine) Start(ctx context.Context) error {
	if e == nil || e.instance == nil {
		return fmt.Errorf("xraycore: engine not initialized")
	}
	return nil
}

// Close stops the xray-core instance and releases its listeners. Idempotent:
// callers may hold both a deferred Close and an explicit one (e.g. to
// verify port release before returning) without a second, noisy
// "already closed" error.
func (e *Engine) Close() error {
	if e == nil || e.instance == nil {
		return nil
	}
	e.closeOnce.Do(func() {
		if err := e.instance.Close(); err != nil {
			e.closeErr = fmt.Errorf("xraycore: Close: %w", err)
		}
	})
	return e.closeErr
}

// DumpConfig optionally writes the built config to config.json for
// troubleshooting — only when DEBUG_MODE is on. It is never required for
// xray-core itself to run (no file round-trip), it's purely a debugging
// aid.
func DumpConfig(path string, cfgBytes []byte, debugMode bool) {
	if !debugMode {
		return
	}
	if err := os.WriteFile(path, cfgBytes, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "[!] xraycore: failed to write debug config dump to %s: %v\n", path, err)
	}
}
