package tunnel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
)

var (
	fakeBinOnce sync.Once
	fakeBinPath string
	fakeBinErr  error
	fakeBinDir  string
)

// TestMain builds testdata/fakecloudflared exactly once into a directory
// that outlives every individual test (t.TempDir() is wrong here — it's
// removed as soon as the *creating* test finishes, but sync.Once only runs
// the build once, so every later test would be handed a already-deleted
// path).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakecloudflared-build")
	if err == nil {
		fakeBinDir = dir
	}
	code := m.Run()
	if fakeBinDir != "" {
		os.RemoveAll(fakeBinDir)
	}
	os.Exit(code)
}

// buildFakeCloudflared compiles testdata/fakecloudflared once per test
// binary run and returns its path — this is the "tiny Go program" fake
// double the plan calls for, so Supervisor's restart logic is exercised
// without any real network/API dependency.
func buildFakeCloudflared(t *testing.T) string {
	t.Helper()
	fakeBinOnce.Do(func() {
		if fakeBinDir == "" {
			fakeBinErr = fmt.Errorf("fakeBinDir not initialized (TestMain build dir creation failed)")
			return
		}
		out := filepath.Join(fakeBinDir, "fakecloudflared")
		cmd := exec.Command("go", "build", "-o", out, "./testdata/fakecloudflared")
		cmd.Dir = "."
		if output, err := cmd.CombinedOutput(); err != nil {
			fakeBinErr = err
			t.Logf("build output: %s", output)
			return
		}
		fakeBinPath = out
	})
	if fakeBinErr != nil {
		t.Fatalf("building fakecloudflared: %v", fakeBinErr)
	}
	return fakeBinPath
}

func testSupervisorConfig(t *testing.T) *config.Config {
	t.Helper()
	ports, err := config.ParsePorts("127.0.0.1:8888")
	if err != nil {
		t.Fatal(err)
	}
	return &config.Config{Ports: ports, TargetIP: "127.0.0.1", TargetPort: 8888, WSHost: "trycloudflare.com"}
}

func TestSupervisorRestartsOnCrash(t *testing.T) {
	bin := buildFakeCloudflared(t)
	workDir := t.TempDir()
	cfg := testSupervisorConfig(t)

	os.Setenv("FAKE_CF_MODE", "quick")
	os.Setenv("FAKE_CF_SUBDOMAIN", "crash-test")
	os.Setenv("FAKE_CF_EXIT_AFTER", "300ms")
	os.Setenv("FAKE_CF_READY_AFTER", "50ms")
	defer os.Unsetenv("FAKE_CF_MODE")
	defer os.Unsetenv("FAKE_CF_SUBDOMAIN")
	defer os.Unsetenv("FAKE_CF_EXIT_AFTER")
	defer os.Unsetenv("FAKE_CF_READY_AFTER")

	sup := NewSupervisor(cfg, bin, workDir)
	var mu sync.Mutex
	var launchCount int
	var lines []string
	sup.LogLine = func(line string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, line)
		if strings.Contains(line, "Restarting") {
			launchCount++
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		sup.Run(ctx)
		close(done)
	}()

	<-done

	mu.Lock()
	defer mu.Unlock()
	if launchCount < 1 {
		t.Errorf("expected at least 1 restart after simulated crash, got %d restarts. lines=%v", launchCount, lines)
	}
}

func TestSupervisorDetectsQuickTunnelHostname(t *testing.T) {
	bin := buildFakeCloudflared(t)
	workDir := t.TempDir()
	cfg := testSupervisorConfig(t)

	os.Setenv("FAKE_CF_MODE", "quick")
	os.Setenv("FAKE_CF_SUBDOMAIN", "hostname-test")
	os.Setenv("FAKE_CF_READY_AFTER", "50ms")
	os.Unsetenv("FAKE_CF_EXIT_AFTER")
	defer os.Unsetenv("FAKE_CF_MODE")
	defer os.Unsetenv("FAKE_CF_SUBDOMAIN")
	defer os.Unsetenv("FAKE_CF_READY_AFTER")

	sup := NewSupervisor(cfg, bin, workDir)
	hostCh := make(chan string, 4)
	sup.OnHostname = func(h string) { hostCh <- h }
	sup.LogLine = func(l string) { t.Log(l) }

	ctx, cancel := context.WithCancel(context.Background())
	runDone := startSupervisor(sup, ctx)
	defer stopAndWaitSupervisor(t, cancel, runDone)

	select {
	case host := <-hostCh:
		if host != "hostname-test.trycloudflare.com" {
			t.Errorf("got hostname %q, want hostname-test.trycloudflare.com", host)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for OnHostname callback")
	}
}

// startSupervisor runs sup.Run(ctx) in a goroutine and returns a channel
// closed when it returns.
func startSupervisor(sup *Supervisor, ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		sup.Run(ctx)
		close(done)
	}()
	return done
}

// stopAndWaitSupervisor cancels and blocks until sup.Run's goroutine has
// actually returned — i.e. the subprocess has actually exited, per
// Supervisor's own bounded graceful-shutdown path — before the caller's
// test function returns. A bare deferred cancel() alone doesn't wait for
// that, and can leak the fake cloudflared subprocess past the end of a
// test (found during the code-review/test pass on this phase).
func stopAndWaitSupervisor(t *testing.T, cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	cancel()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Error("Supervisor.Run did not return within 8s of cancellation; subprocess may have leaked")
	}
}

func TestSupervisorNamedTunnelZeroLogParsing(t *testing.T) {
	// Named-tunnel path must never depend on stdout parsing for hostname
	// discovery — verify OnHostname fires with the configured WSHost (not
	// something scraped from a log line) once /ready reports connected.
	bin := buildFakeCloudflared(t)
	workDir := t.TempDir()
	cfg := testSupervisorConfig(t)
	cfg.TunnelToken = "fake-token-for-test"
	cfg.WSHost = "reverse.example.io.vn"

	os.Setenv("FAKE_CF_MODE", "named")
	os.Setenv("FAKE_CF_READY_AFTER", "50ms")
	os.Unsetenv("FAKE_CF_EXIT_AFTER")
	os.Unsetenv("FAKE_CF_SUBDOMAIN")
	defer os.Unsetenv("FAKE_CF_MODE")
	defer os.Unsetenv("FAKE_CF_READY_AFTER")

	sup := NewSupervisor(cfg, bin, workDir)
	hostCh := make(chan string, 4)
	sup.OnHostname = func(h string) { hostCh <- h }

	ctx, cancel := context.WithCancel(context.Background())
	runDone := startSupervisor(sup, ctx)
	defer stopAndWaitSupervisor(t, cancel, runDone)

	select {
	case host := <-hostCh:
		if host != "reverse.example.io.vn" {
			t.Errorf("got hostname %q, want reverse.example.io.vn (from WSHost, not stdout)", host)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for OnHostname callback")
	}

	if _, err := os.Stat(filepath.Join(workDir, "config.yml")); err != nil {
		t.Errorf("expected config.yml to be written for named tunnel: %v", err)
	}
}

// TestSupervisorGracefulShutdownWaitsForExit guards a real finding from
// code review: Run() previously returned as soon as ctx was cancelled
// without waiting for the subprocess to actually exit, racing the parent
// process's own shutdown and risking an orphaned cloudflared. A
// well-behaved process (the common case: it honors SIGINT) should make
// Run() return quickly, proving the wait is for real exit, not just a
// fixed sleep.
func TestSupervisorGracefulShutdownWaitsForExit(t *testing.T) {
	bin := buildFakeCloudflared(t)
	workDir := t.TempDir()
	cfg := testSupervisorConfig(t)

	os.Setenv("FAKE_CF_MODE", "quick")
	os.Setenv("FAKE_CF_SUBDOMAIN", "shutdown-test")
	os.Unsetenv("FAKE_CF_EXIT_AFTER")
	os.Unsetenv("FAKE_CF_IGNORE_SIGINT")
	defer os.Unsetenv("FAKE_CF_MODE")
	defer os.Unsetenv("FAKE_CF_SUBDOMAIN")

	sup := NewSupervisor(cfg, bin, workDir)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		sup.Run(ctx)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond) // let it actually launch
	start := time.Now()
	cancel()

	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("Run() took %s to return after a process that honors SIGINT; expected well under shutdownGrace (5s)", elapsed)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Run() did not return within 6s of ctx cancellation")
	}
}

// TestSupervisorOnReadyFiresOnReadyPollAndChannelClose exercises watch()
// directly (package-internal call, no real subprocess needed) to cover the
// red-team-identified stale-status gap: OnReady must fire on every
// successful /ready poll outcome AND once more with Ready:false when the
// ready-poll channel closes ("!ok" early-return path) — not just on the
// happy-path poll.
func TestSupervisorOnReadyFiresOnReadyPollAndChannelClose(t *testing.T) {
	cfg := testSupervisorConfig(t)
	sup := NewSupervisor(cfg, "unused-bin", t.TempDir())

	var mu sync.Mutex
	var states []ReadyState
	sup.OnReady = func(s ReadyState) {
		mu.Lock()
		defer mu.Unlock()
		states = append(states, s)
	}

	readyCh := make(chan ReadyState, 1)
	readyCh <- ReadyState{Ready: true, ReadyConnections: 3}
	close(readyCh) // next receive returns ok=false, driving the "!ok" path

	exitCh := make(chan error) // never fires — forces the readyCh branch
	handle := &Handle{Named: false}

	reason := sup.watch(context.Background(), exitCh, readyCh, handle)
	if reason != "ready-poll channel closed" {
		t.Errorf("watch() reason = %q, want %q", reason, "ready-poll channel closed")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(states) != 2 {
		t.Fatalf("expected 2 OnReady calls (one ready poll + one channel-close), got %d: %+v", len(states), states)
	}
	if !states[0].Ready || states[0].ReadyConnections != 3 {
		t.Errorf("first OnReady = %+v, want Ready:true ReadyConnections:3 (the polled state)", states[0])
	}
	if states[1].Ready {
		t.Errorf("second OnReady = %+v, want Ready:false (the channel-close fallback)", states[1])
	}
}

// TestSupervisorOnReadyFiresOnProcessExit covers watch()'s exitCh
// early-return path: a crash must also report Ready:false, not just leave
// the dashboard on the last-known (possibly stale "ready") state.
func TestSupervisorOnReadyFiresOnProcessExit(t *testing.T) {
	cfg := testSupervisorConfig(t)
	sup := NewSupervisor(cfg, "unused-bin", t.TempDir())

	var mu sync.Mutex
	var states []ReadyState
	sup.OnReady = func(s ReadyState) {
		mu.Lock()
		defer mu.Unlock()
		states = append(states, s)
	}

	exitCh := make(chan error, 1)
	exitCh <- fmt.Errorf("boom")
	var readyCh chan ReadyState // nil — never fires, forces the exitCh branch
	handle := &Handle{Named: false}

	reason := sup.watch(context.Background(), exitCh, readyCh, handle)
	if !strings.Contains(reason, "boom") {
		t.Errorf("watch() reason = %q, want it to contain %q", reason, "boom")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(states) != 1 || states[0].Ready {
		t.Fatalf("expected exactly 1 OnReady(Ready:false) call, got %+v", states)
	}
}

// TestSupervisorGracefulShutdownEscalatesToKill guards the Kill()
// escalation path: a subprocess that does NOT honor a graceful Terminate()
// must still be forced to exit (and Run() must still return) within
// shutdownGrace, not hang forever.
func TestSupervisorGracefulShutdownEscalatesToKill(t *testing.T) {
	bin := buildFakeCloudflared(t)
	workDir := t.TempDir()
	cfg := testSupervisorConfig(t)

	os.Setenv("FAKE_CF_MODE", "quick")
	os.Setenv("FAKE_CF_SUBDOMAIN", "kill-test")
	os.Setenv("FAKE_CF_IGNORE_SIGINT", "1")
	os.Unsetenv("FAKE_CF_EXIT_AFTER")
	defer os.Unsetenv("FAKE_CF_MODE")
	defer os.Unsetenv("FAKE_CF_SUBDOMAIN")
	defer os.Unsetenv("FAKE_CF_IGNORE_SIGINT")

	sup := NewSupervisor(cfg, bin, workDir)
	sup.LogLine = func(l string) { t.Log(l) }
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		sup.Run(ctx)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Run() returning at all proves the Kill() escalation fired and
		// the process actually exited — a process ignoring SIGINT that was
		// never escalated to Kill() would hang this test at the 8s
		// deadline below instead.
	case <-time.After(8 * time.Second):
		t.Fatal("Run() did not return within 8s for a SIGINT-ignoring process; Kill() escalation may be missing or broken")
	}
}
