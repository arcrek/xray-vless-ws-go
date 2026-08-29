package tunnel

import (
	"context"
	"fmt"
	"time"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
)

const (
	readyPollInterval = 2 * time.Second
	notReadyGrace     = 20 * time.Second // sustained not-ready-while-alive before a restart
	backoffInitial    = 2 * time.Second
	backoffMax        = 60 * time.Second
	shutdownGrace     = 5 * time.Second // how long to wait for cloudflared to exit after Terminate() before escalating to Kill()
)

// Supervisor owns cloudflared's restart loop — process-exit OR
// sustained-not-ready — for the cloudflared half only (xray-core's own
// lifecycle, Phase 2, doesn't need this kind of supervision since it's
// in-process).
type Supervisor struct {
	cfg     *config.Config
	binPath string
	workDir string

	// LogLine receives every cloudflared stdout/stderr line plus
	// supervisor-generated events (restarts, new hostnames).
	LogLine func(line string)

	// OnHostname fires once per detected/changed tunnel hostname: for a
	// quick tunnel, when a new *.trycloudflare.com domain is parsed from
	// stdout; for a named tunnel, once when /ready first reports a
	// connection (WS_HOST is already known, no stdout parsing involved —
	// a stronger zero-log-parsing guarantee than matching a
	// "registered tunnel connection" log line).
	OnHostname func(hostname string)
}

// NewSupervisor constructs a Supervisor. binPath is the cloudflared binary
// path (see EnsureBinary); workDir is where config.yml is written for named
// tunnels.
func NewSupervisor(cfg *config.Config, binPath, workDir string) *Supervisor {
	return &Supervisor{cfg: cfg, binPath: binPath, workDir: workDir}
}

func (s *Supervisor) log(format string, args ...any) {
	if s.LogLine != nil {
		s.LogLine(fmt.Sprintf(format, args...))
	}
}

// Run launches cloudflared and supervises it until ctx is cancelled. It
// never returns until ctx is done (or a non-recoverable launch error
// occurs on the very first attempt).
func (s *Supervisor) Run(ctx context.Context) error {
	backoff := backoffInitial

	for {
		if ctx.Err() != nil {
			return nil
		}

		handle, err := Launch(s.cfg, s.binPath, s.workDir)
		if err != nil {
			s.log("[!] Failed to launch cloudflared: %v", err)
			if !sleepOrDone(ctx, backoff) {
				return nil
			}
			backoff = nextBackoff(backoff)
			continue
		}

		runCtx, cancelRun := context.WithCancel(ctx)
		exitCh := make(chan error, 1)
		go func() { exitCh <- handle.Wait() }()

		go s.streamLines(handle)

		readyCh := PollReady(runCtx, handle.MetricsAddr, readyPollInterval)
		restartReason := s.watch(runCtx, exitCh, readyCh, handle)
		cancelRun()
		// No drain of exitCh here: it's buffered (cap 1) and fed by exactly
		// one send from the `go func(){ exitCh <- handle.Wait() }()` above,
		// which completes (and that goroutine exits) whether or not
		// anything ever reads it. watch() already consumed the value when
		// it returned via the exitCh case; reading again here would block
		// forever in that path since nothing sends a second time.

		if ctx.Err() != nil {
			_ = handle.Terminate()
			return nil
		}

		s.log("[!] cloudflared %s. Restarting...", restartReason)
		handle.Kill()

		if !sleepOrDone(ctx, backoff) {
			return nil
		}
		backoff = nextBackoff(backoff)
	}
}

// watch blocks until the process exits or has been sustained-not-ready for
// longer than notReadyGrace, returning a human-readable reason. It also
// drives hostname detection for both tunnel kinds.
func (s *Supervisor) watch(ctx context.Context, exitCh <-chan error, readyCh <-chan ReadyState, handle *Handle) string {
	var notReadySince time.Time
	namedHostnameFired := false

	for {
		select {
		case <-ctx.Done():
			// Graceful shutdown: signal the subprocess and actually wait
			// (bounded) for it to exit before returning, so Run()'s caller
			// (main.go, via defer/ctx.Done()) never returns while
			// cloudflared is still alive — an un-awaited Terminate() alone
			// races the parent process's own exit and can orphan the
			// subprocess (reparented to init) if the parent wins that race.
			_ = handle.Terminate()
			select {
			case <-exitCh:
			case <-time.After(shutdownGrace):
				s.log("[!] cloudflared did not exit within %s of shutdown signal, killing", shutdownGrace)
				handle.Kill()
				<-exitCh
			}
			return "context cancelled"

		case err := <-exitCh:
			if err != nil {
				return fmt.Sprintf("process exited unexpectedly: %v", err)
			}
			return "process exited unexpectedly"

		case state, ok := <-readyCh:
			if !ok {
				return "ready-poll channel closed"
			}
			if state.Err != nil {
				s.log("[!] /ready poll error: %v", state.Err)
				continue
			}
			if state.Ready {
				notReadySince = time.Time{}
				if handle.Named && !namedHostnameFired && s.OnHostname != nil {
					namedHostnameFired = true
					s.OnHostname(s.cfg.WSHost)
				}
				continue
			}
			if notReadySince.IsZero() {
				notReadySince = time.Now()
				continue
			}
			if time.Since(notReadySince) > notReadyGrace {
				return fmt.Sprintf("not ready for over %s", notReadyGrace)
			}
		}
	}
}

// streamLines reads handle's combined stdout/stderr, forwards every line to
// LogLine, and (for quick tunnels only) feeds HostnameParser to detect a
// new/changed *.trycloudflare.com domain.
func (s *Supervisor) streamLines(handle *Handle) {
	parser := NewHostnameParser()
	scanner := handle.Lines()
	for scanner.Scan() {
		line := scanner.Text()
		s.log("%s", StripANSI(line))

		if handle.Named {
			continue
		}
		if host, found := parser.Feed(line); found && s.OnHostname != nil {
			s.OnHostname(host)
		}
	}
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

func nextBackoff(cur time.Duration) time.Duration {
	next := cur * 2
	if next > backoffMax {
		return backoffMax
	}
	return next
}
