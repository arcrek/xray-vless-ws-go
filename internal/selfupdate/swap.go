package selfupdate

import (
	"fmt"
	"os"
)

// atomicReplace preserves execPath as execPath+".prev" (a hardlink, not a
// copy — free, instant, same inode until the next update overwrites the
// link) so --rollback has a single-level fallback, then atomically swaps
// tmpPath into execPath's place. tmpPath and execPath must already be on
// the same filesystem for os.Rename to be atomic — callers guarantee this
// by creating tmpPath via os.CreateTemp(filepath.Dir(execPath), ...) (see
// run.go's Run), never os.TempDir().
//
// All failure paths return before os.Rename, leaving execPath untouched —
// a failed update always leaves the previous, working binary in place. If
// preserving the rollback hardlink itself fails (step 2), that's treated
// as a hard failure and the swap is aborted before touching execPath at
// all — better to fail the update cleanly than silently ship one with no
// rollback safety net.
//
// The deferred os.Remove(tmpPath) is a best-effort no-op after a
// successful rename (Rename already consumed tmpPath); it only cleans up
// a leftover partial file on an early-return failure path.
func atomicReplace(execPath, tmpPath string) error {
	defer os.Remove(tmpPath)

	prevPath := execPath + ".prev"
	if err := os.Remove(prevPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("selfupdate: removing stale %s: %w", prevPath, err)
	}

	if err := os.Link(execPath, prevPath); err != nil {
		return fmt.Errorf("selfupdate: preserving rollback copy at %s: %w", prevPath, err)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("selfupdate: chmod +x on new binary: %w", err)
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		return fmt.Errorf("selfupdate: swapping in new binary: %w", err)
	}

	return nil
}
