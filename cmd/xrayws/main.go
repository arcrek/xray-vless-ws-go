// Command xrayws is the production entrypoint: it loads config (Phase 1),
// starts the embedded xray-core engine (Phase 2), launches and supervises
// cloudflared (Phase 3), exports vless:// links on every hostname change
// (Phase 4), starts the optional log viewer (Phase 5), and branches into
// --ci-mode (Phase 6) if that flag is set. This file is orchestration
// only — no business logic lives here, it all lives in the internal
// packages above.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/arcrek/xray-vless-ws-go/internal/cfdeploy"
	"github.com/arcrek/xray-vless-ws-go/internal/ci"
	"github.com/arcrek/xray-vless-ws-go/internal/config"
	"github.com/arcrek/xray-vless-ws-go/internal/linkgen"
	"github.com/arcrek/xray-vless-ws-go/internal/logserver"
	"github.com/arcrek/xray-vless-ws-go/internal/tunnel"
	"github.com/arcrek/xray-vless-ws-go/internal/xraycore"
)

func main() {
	ciMode := flag.Bool("ci-mode", false, "Run the GitHub Actions CI bridge alongside the proxy (export ENV_CONFIG, watch+upload frp_info.*, self re-dispatch, self-exit before the 6h job limit)")
	logPort := flag.Int("log-port", 9999, "Port for the embedded log viewer (0 disables it)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *ciMode, *logPort); err != nil {
		fmt.Fprintf(os.Stderr, "[!] fatal: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, ciMode bool, logPort int) error {
	if ciMode {
		// Ported from github-workflows.py's run order: export the secret
		// BEFORE config.Load() reads .env, so a CI-injected ENV_CONFIG
		// takes effect on the very first load, not just on a later Reload.
		if err := ci.ExportSecretEnv(".env"); err != nil {
			return fmt.Errorf("ci: %w", err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Cloudflare Worker auto-deploy (internal/cfdeploy): no-op unless both
	// CLOUDFLARE_API_TOKEN and DOMAIN are set in .env. Runs before anything
	// else starts so cfg.WebhookURL is final by the time exportLinks first
	// fires. Provisioning failure is non-fatal — same "independently
	// non-fatal" pattern as webhook delivery itself (exportLinks below):
	// cfg.WebhookURL is simply left as whatever .env already had.
	if webhookURL, err := cfdeploy.Ensure(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Cloudflare auto-deploy failed: %v — falling back to WEBHOOK_URL from .env\n", err)
	} else if webhookURL != "" {
		cfg.WebhookURL = webhookURL
	}

	startTime := time.Now().Unix()

	// wg tracks every goroutine that owns a subprocess or a listener, so
	// shutdown can actually wait for them instead of returning from run()
	// (and letting the process exit) while cloudflared or the log server
	// are still mid-teardown — an un-awaited goroutine here previously
	// raced the parent process's own exit and could orphan cloudflared.
	var wg sync.WaitGroup

	var logSrv *logserver.Server
	if logPort > 0 {
		logSrv = logserver.New(fmt.Sprintf("0.0.0.0:%d", logPort), cfg.LogPassword, 500)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := logSrv.Start(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "[!] log server stopped: %v\n", err)
			}
		}()
		fmt.Printf("[*] Logger Web UI is running at: http://localhost:%d\n", logPort)
	}
	// alwaysPrint mirrors main.py's actual (asymmetric) behavior: xray's own
	// log lines are DEBUG_MODE-gated on stdout (main.py:290), but
	// monitor_cloudflare's lines print unconditionally (main.py:315) — that
	// asymmetry matters in practice, since a cloudflared launch failure is
	// an operational signal, not routine chatter, and burying it behind a
	// flag most users don't set made a real launch failure invisible on
	// stdout in testing (only visible via the log web UI). Both sources
	// still always go to the log server regardless.
	logf := func(source string, alwaysPrint bool) func(string) {
		return func(line string) {
			if logSrv != nil {
				logSrv.Push(source, line)
			}
			if alwaysPrint || cfg.DebugMode {
				fmt.Printf("[%s] %s\n", source, line)
			}
		}
	}

	// --- Phase 2: embedded xray-core engine ---
	xrayCfgBytes, err := xraycore.BuildConfig(cfg)
	if err != nil {
		return fmt.Errorf("xraycore: %w", err)
	}
	xraycore.DumpConfig("config.json", xrayCfgBytes, cfg.DebugMode)

	fmt.Println("[*] Launching XRAY with multi-port inbounds...")
	engine, err := xraycore.New(xrayCfgBytes, logf("XRAY", false))
	if err != nil {
		return fmt.Errorf("xraycore: %w", err)
	}
	defer engine.Close()

	// --- Phase 3: cloudflared, downloaded + supervised ---
	binPath, err := tunnel.EnsureBinary(ctx, ".")
	if err != nil {
		return fmt.Errorf("tunnel: %w", err)
	}

	sup := tunnel.NewSupervisor(cfg, binPath, ".")
	sup.LogLine = logf("CLOUDFLARE", true)
	sup.OnHostname = func(hostname string) {
		exportLinks(ctx, cfg, hostname, startTime)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		sup.Run(ctx)
	}()

	// --- Phase 6: CI bridge, only under --ci-mode ---
	if ciMode {
		token := os.Getenv("GITHUB_TOKEN")
		repo := os.Getenv("GITHUB_REPOSITORY")
		go ci.WatchAndUpload(ctx, token, repo,
			[]string{"frp_info.config", "frp_info.json"}, "config", "__tmp__",
			logf("CI", true))
		ci.ScheduleBridge(ctx, logf("CI", true))
		ci.ScheduleSelfExit(ctx, func() {
			fmt.Println("[*] CI self-exit timer elapsed, exiting.")
			os.Exit(0)
		})
	}

	<-ctx.Done()
	fmt.Println("\n[*] Stopping services...")

	// Wait for the supervisor (which owns the cloudflared subprocess) and
	// the log server to actually finish tearing down before returning —
	// bounded, so a genuinely stuck goroutine can't hang shutdown forever;
	// tunnel.Supervisor's own shutdownGrace (5s) should make this return
	// well within the deadline in the normal case.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		fmt.Fprintln(os.Stderr, "[!] shutdown did not complete within 10s, exiting anyway")
	}

	return nil
}

// exportLinks rebuilds and writes vless:// links whenever the tunnel
// hostname is first detected or changes, mirroring main.py's
// monitor_cloudflare() calling print_vless_links() on every new hostname
// (including quick-tunnel reconnects). Public IP lookup, file export, and
// webhook delivery are each independently non-fatal (Phase 4 requirement).
func exportLinks(ctx context.Context, cfg *config.Config, tunnelHost string, startTime int64) {
	fmt.Println("\n" + repeatEquals(70))
	fmt.Println(" CONNECTED TO CLOUDFLARE TUNNEL")
	fmt.Println(repeatEquals(70))

	links := linkgen.BuildLinks(cfg, cfg.XrayUUID, tunnelHost)
	if cfg.DebugMode {
		for _, l := range links {
			fmt.Println(l)
		}
	}

	ip := linkgen.LookupPublicIP(ctx)
	meta := linkgen.ExportMeta{
		IP:        ip,
		WSHost:    tunnelHost,
		WSPath:    cfg.WSPath,
		Transport: cfg.Transport,
		StartTime: startTime,
	}

	if err := linkgen.Export(links, meta, "frp_info.config", "frp_info.json"); err != nil {
		fmt.Fprintf(os.Stderr, "[!] linkgen export error: %v\n", err)
	}

	if cfg.WebhookURL != "" {
		linkgen.SendWebhook(cfg.WebhookURL, map[string]any{
			"payloads":   links,
			"ip":         ip,
			"wshost":     tunnelHost,
			"wspath":     cfg.WSPath,
			"transport":  cfg.Transport,
			"start_time": startTime,
		})
	}
}

func repeatEquals(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '='
	}
	return string(b)
}
