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

// osExecutable is overridable in tests so Run/Rollback can be exercised
// against a fake t.TempDir() binary path instead of the real test binary
// — same injectable-var idiom as execCommand/isUnitPresent in restart.go.
var osExecutable = os.Executable

// Run checks the latest GitHub Release against currentVersion, and if
// newer, downloads the matching platform asset, verifies it against
// SHA256SUMS, atomically installs it (preserving the previous binary for
// --rollback), and restarts the xrayws systemd service. Progress is
// narrated to out, mirroring install.sh's own narrated-steps UX. Returns a
// non-nil error on any failure, including the partial-success case where
// the binary swap succeeded but the restart didn't (Decision 5) — main
// exits non-zero either way so the failure is visible.
func Run(ctx context.Context, currentVersion string, out io.Writer) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("self-update while running isn't supported on Windows — download the new build manually")
	}

	fmt.Fprintln(out, "[*] Checking latest release...")

	// os.Executable's own doc warns the result may be a symlink; resolve it
	// so execDir/execPath refer to the binary's real location.
	execPath, err := osExecutable()
	if err != nil {
		return fmt.Errorf("selfupdate: locating running binary: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("selfupdate: resolving symlinks for %s: %w", execPath, err)
	}

	tag, assets, err := latestRelease(ctx)
	if err != nil {
		return err
	}

	if currentVersion == "dev" {
		fmt.Fprintf(out, "[*] Running a dev build, cannot compare versions — proceeding with update to %s\n", tag)
	} else if currentVersion == tag {
		fmt.Fprintf(out, "[+] Already up to date (%s)\n", currentVersion)
		return nil
	} else {
		fmt.Fprintf(out, "[*] Current: %s, latest: %s\n", currentVersion, tag)
	}

	assetName := fmt.Sprintf("xrayws-%s-%s", runtime.GOOS, runtime.GOARCH)
	asset, ok := assets[assetName]
	if !ok {
		return fmt.Errorf("selfupdate: no release asset named %s found in release %s", assetName, tag)
	}
	sums, ok := assets["SHA256SUMS"]
	if !ok {
		return fmt.Errorf("selfupdate: no SHA256SUMS asset found in release %s", tag)
	}

	// Write-permission preflight (Decision 5): positioned in execDir (not
	// os.TempDir()) so the later os.Rename in atomicReplace is guaranteed
	// same-filesystem, hence atomic. Creating this temp file *is* the
	// preflight check — failing here, before any download, is exactly the
	// "fail fast" behavior the plan calls for, and the real os.CreateTemp
	// error (permission denied, read-only filesystem, disk full, ...) is
	// preserved in the message rather than assumed to always be a
	// permission problem.
	execDir := filepath.Dir(execPath)
	tmpFile, err := os.CreateTemp(execDir, "xrayws-update-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w — if this is a permission error, re-run with sudo (the systemd deployment in install.sh normally runs as root)", execDir, err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	fmt.Fprintf(out, "[*] Downloading %s...\n", assetName)
	if err := downloadAndVerify(ctx, asset.DownloadURL, sums.DownloadURL, assetName, tmpPath); err != nil {
		os.Remove(tmpPath)
		return err
	}
	fmt.Fprintln(out, "[+] Checksum verified")

	if err := atomicReplace(execPath, tmpPath); err != nil {
		return err
	}
	fmt.Fprintln(out, "[+] Binary replaced")

	fmt.Fprintln(out, "[*] Restarting xrayws.service...")
	if err := restartService(ctx); err != nil {
		if errors.Is(err, errNoSystemdUnit) {
			fmt.Fprintf(out, "[+] Binary updated to %s — no systemd unit detected, restart the process manually to run the new version\n", tag)
			return nil
		}
		return fmt.Errorf("binary updated to %s, but automatic restart failed: %w — restart manually: sudo systemctl restart %s", tag, err, serviceName)
	}

	fmt.Fprintf(out, "[+] Updated %s -> %s and restarted xrayws.service\n", currentVersion, tag)
	return nil
}

// checkWritable performs a preflight write-access check on dir by creating
// and immediately removing a throwaway file — used by Rollback, which
// (unlike Run) has no download step of its own to double as the check.
// Fails fast, before the rename, rather than discovering the permission
// problem only when os.Rename itself fails.
func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".xrayws-write-check-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return nil
}
