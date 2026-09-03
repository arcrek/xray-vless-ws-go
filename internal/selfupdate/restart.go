package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// serviceName is the fixed systemd unit name install.sh creates
// (SERVICE_NAME="xrayws", lines 259-260) — not configurable, matching the
// rest of this repo's "fixed, not configurable" bias for install-time
// constants.
const serviceName = "xrayws"

// errNoSystemdUnit is a sentinel restartService returns when no xrayws
// systemd unit is detected. Callers (Run, Rollback) treat this as a clean,
// non-fatal condition (Decision 6) — the binary swap already succeeded,
// there's just no service to restart automatically.
var errNoSystemdUnit = errors.New("no systemd unit detected")

// execCommand is overridable in tests so restart_test.go/run_test.go can
// inject a fake instead of actually shelling out — same "injectable func
// field" idiom as tunnel.Supervisor's LogLine/OnHostname/OnReady.
var execCommand = exec.Command

// isUnitPresent is overridable in tests; production checks whether the
// unit *file* exists via LoadState, not whether it's enabled/active —
// `systemctl is-enabled` alone would also report a false "not present" for
// a unit that's installed and running but was explicitly disabled (e.g. an
// operator ran `systemctl disable xrayws` to stop it auto-starting on
// reboot while leaving it running), which would wrongly skip a working
// restart. LoadState is "loaded" whenever the unit file resolves, "not-found"
// when it doesn't, regardless of enabled/active state.
var isUnitPresent = func(ctx context.Context) bool {
	cmd := execCommand("systemctl", "show", "-p", "LoadState", "--value", serviceName+".service")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "loaded"
}

// buildRestartCommand returns the systemctl restart argv, prefixed with
// sudo when the current process isn't root — mirrors install.sh's own
// do_sudo pattern (id -u check at lines 262-265). Pulled out as a pure
// function so it's unit-testable without shelling out.
func buildRestartCommand(isRoot bool) []string {
	if isRoot {
		return []string{"systemctl", "restart", serviceName}
	}
	return []string{"sudo", "systemctl", "restart", serviceName}
}

// restartService restarts the xrayws systemd service. Returns
// errNoSystemdUnit if no such unit is detected (e.g. a manual foreground
// run, or install.sh's no-systemd fallback) — the caller decides how to
// present that.
func restartService(ctx context.Context) error {
	if !isUnitPresent(ctx) {
		return errNoSystemdUnit
	}

	argv := buildRestartCommand(os.Geteuid() == 0)
	cmd := execCommand(argv[0], argv[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %w: %s", argv, err, out)
	}
	return nil
}
