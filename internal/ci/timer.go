package ci

import (
	"context"
	"time"
)

// selfExitAfter is 5h40m, matching github-workflows.py:114's
// `time.sleep(5*60*60 + 40*60); os._exit(0)` — a hard self-exit safety net
// so the process never runs past the GitHub Actions 6h job limit even if
// nothing else stops it first.
const selfExitAfter = 5*time.Hour + 40*time.Minute

// ScheduleSelfExit calls onExit after selfExitAfter, unless ctx is
// cancelled first. Kept as a callback (rather than calling os.Exit
// directly) so main.go controls the actual shutdown sequence — e.g.
// draining logs or closing the xray-core engine — rather than this package
// hard-killing the process itself.
func ScheduleSelfExit(ctx context.Context, onExit func()) {
	go func() {
		select {
		case <-time.After(selfExitAfter):
			onExit()
		case <-ctx.Done():
		}
	}()
}
