package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// Rollback swaps the preserved pre-update binary (execPath+".prev") back
// into place and restarts the xrayws systemd service. Single-level only:
// a successful rollback consumes .prev via os.Rename, so running
// --rollback twice in a row fails cleanly on the second attempt. No
// download/verify is needed — .prev is a hardlink to bytes that were
// already the verified, previously-running binary.
func Rollback(ctx context.Context, out io.Writer) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("self-update while running isn't supported on Windows — download the new build manually")
	}

	execPath, err := osExecutable()
	if err != nil {
		return fmt.Errorf("selfupdate: locating running binary: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("selfupdate: resolving symlinks for %s: %w", execPath, err)
	}
	prevPath := execPath + ".prev"

	if _, err := os.Stat(prevPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no previous binary to roll back to (run --update at least once first)")
		}
		return fmt.Errorf("selfupdate: checking %s: %w", prevPath, err)
	}

	execDir := filepath.Dir(execPath)
	if err := checkWritable(execDir); err != nil {
		return fmt.Errorf("cannot write to %s: %w — if this is a permission error, re-run with sudo (the systemd deployment in install.sh normally runs as root)", execDir, err)
	}

	if err := os.Rename(prevPath, execPath); err != nil {
		return fmt.Errorf("selfupdate: rolling back to previous binary: %w", err)
	}
	fmt.Fprintln(out, "[+] Rolled back to previous binary")

	fmt.Fprintln(out, "[*] Restarting xrayws.service...")
	if err := restartService(ctx); err != nil {
		if errors.Is(err, errNoSystemdUnit) {
			fmt.Fprintln(out, "[+] Rollback complete — no systemd unit detected, restart the process manually to run the previous version")
			return nil
		}
		return fmt.Errorf("rolled back, but automatic restart failed: %w — restart manually: sudo systemctl restart %s", err, serviceName)
	}

	fmt.Fprintln(out, "[+] Rollback complete and restarted xrayws.service")
	return nil
}
