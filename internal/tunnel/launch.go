package tunnel

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
)

// Handle wraps one running cloudflared subprocess.
type Handle struct {
	Cmd         *exec.Cmd
	MetricsAddr string
	Named       bool // true if this is a named-tunnel launch (no hostname parsing needed)

	stdout io.ReadCloser
}

// Lines returns a scanner over the subprocess's combined stdout+stderr, one
// call site (the log-forwarding goroutine) owns reading it.
func (h *Handle) Lines() *bufio.Scanner {
	return bufio.NewScanner(h.stdout)
}

// freeLocalPort asks the OS for a free TCP port on 127.0.0.1 by binding to
// port 0 and immediately releasing it — the standard "pick a free port"
// trick. A random free port per launch avoids collisions across restarts
// within one run and across multiple concurrent instances on the same host.
func freeLocalPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// Launch starts cloudflared, quick-tunnel or named-tunnel depending on
// whether cfg.TunnelToken is set. Every launch gets a
// `--metrics 127.0.0.1:<random-free-port>` flag so PollReady has something
// to poll.
func Launch(cfg *config.Config, binPath, workDir string) (*Handle, error) {
	metricsPort, err := freeLocalPort()
	if err != nil {
		return nil, fmt.Errorf("tunnel: picking a free metrics port: %w", err)
	}
	metricsAddr := fmt.Sprintf("127.0.0.1:%d", metricsPort)

	var cmd *exec.Cmd
	named := cfg.TunnelToken != ""

	if named {
		configPath := filepath.Join(workDir, "config.yml")
		if err := WriteNamedTunnelConfig(configPath, cfg.WSHost, cfg.TargetIP, cfg.TargetPort); err != nil {
			return nil, fmt.Errorf("tunnel: writing config.yml: %w", err)
		}
		fmt.Println("[*] Launching Cloudflare Tunnel (named tunnel via config.yml)...")
		// --token authenticates the tunnel; config.yml supplies only the
		// ingress rules (see WriteNamedTunnelConfig's doc comment for why
		// the token must NOT go in config.yml's `tunnel:` field).
		cmd = exec.Command(binPath, "tunnel",
			"--protocol", "http2",
			"--no-autoupdate",
			"--edge-ip-version", "auto",
			"--grace-period", "30s",
			"--config", configPath,
			"--metrics", metricsAddr,
			"run", "--token", cfg.TunnelToken)
	} else {
		fmt.Printf("[*] Launching Cloudflare Tunnel pointing to http://%s:%d...\n", cfg.TargetIP, cfg.TargetPort)
		cmd = exec.Command(binPath, "tunnel",
			"--protocol", "http2",
			"--no-autoupdate",
			"--edge-ip-version", "auto",
			"--grace-period", "30s",
			"--metrics", metricsAddr,
			"--url", fmt.Sprintf("http://%s:%d", cfg.TargetIP, cfg.TargetPort))
	}
	cmd.Dir = workDir

	// Combine stdout+stderr into one stream — build the pipe manually
	// (rather than cmd.StdoutPipe()) so both cmd.Stdout and cmd.Stderr can
	// point at the same write end.
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("tunnel: creating stdout pipe: %w", err)
	}
	cmd.Stdout = stdoutW
	cmd.Stderr = stdoutW

	if err := cmd.Start(); err != nil {
		stdoutR.Close()
		stdoutW.Close()
		return nil, fmt.Errorf("tunnel: starting cloudflared: %w", err)
	}
	// The write end must be closed in the parent process too, or reads on
	// stdoutR never see EOF after the child exits (the child's copy of the
	// fd keeps it open until then, but the parent's own copy would too).
	stdoutW.Close()

	return &Handle{Cmd: cmd, MetricsAddr: metricsAddr, Named: named, stdout: stdoutR}, nil
}

// Wait blocks until the subprocess exits and returns its error (nil on
// clean exit).
func (h *Handle) Wait() error {
	return h.Cmd.Wait()
}

// Terminate sends SIGTERM (or the OS equivalent) to the subprocess.
func (h *Handle) Terminate() error {
	if h.Cmd.Process == nil {
		return nil
	}
	return h.Cmd.Process.Signal(os.Interrupt)
}

// Kill force-kills the subprocess.
func (h *Handle) Kill() error {
	if h.Cmd.Process == nil {
		return nil
	}
	return h.Cmd.Process.Kill()
}
