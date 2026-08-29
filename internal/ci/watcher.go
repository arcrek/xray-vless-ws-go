package ci

import (
	"context"
	"fmt"
	"os"
	"time"
)

const (
	watchPollInterval  = 2 * time.Second
	uploadDebounce     = 3 * time.Second
	uploadRetries      = 3
	uploadRetryBackoff = 2 * time.Second
)

type fileState struct {
	lastMtime        time.Time
	haveLastMtime    bool
	lastUploadedTime time.Time
}

// LogFunc receives one human-readable watcher event line.
type LogFunc func(line string)

// WatchAndUpload polls paths for mtime changes every watchPollInterval,
// uploading a changed file to branch after uploadDebounce has elapsed since
// its last upload, retrying up to uploadRetries times with
// uploadRetryBackoff between attempts. Blocks until ctx is cancelled.
func WatchAndUpload(ctx context.Context, token, repo string, paths []string, branch, tempDir string, log LogFunc) {
	if log == nil {
		log = func(string) {}
	}
	if token == "" || repo == "" {
		log("Missing GITHUB_TOKEN or GITHUB_REPOSITORY")
		return
	}

	states := make(map[string]*fileState, len(paths))
	for _, p := range paths {
		states[p] = &fileState{}
	}

	log(fmt.Sprintf("Starting to watch files: %v (uploading to '%s')...", paths, branch))

	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, p := range paths {
				checkAndUpload(ctx, token, repo, p, branch, tempDir, states[p], log)
			}
		}
	}
}

func checkAndUpload(ctx context.Context, token, repo, path, branch, tempDir string, state *fileState, log LogFunc) {
	info, err := os.Stat(path)
	if err != nil {
		return // file doesn't exist yet — tolerated, not an error
	}

	mtime := info.ModTime()
	changed := !state.haveLastMtime || !mtime.Equal(state.lastMtime)
	state.lastMtime = mtime
	state.haveLastMtime = true

	if !changed {
		return
	}
	if time.Since(state.lastUploadedTime) <= uploadDebounce {
		return
	}

	log(fmt.Sprintf("[%s] File changed or created!", path))

	for attempt := 1; attempt <= uploadRetries; attempt++ {
		if _, err := UploadFile(ctx, token, repo, path, branch, "", tempDir); err != nil {
			log(fmt.Sprintf("[%s] Upload retry %d/%d failed: %v", path, attempt, uploadRetries, err))
			if attempt < uploadRetries {
				select {
				case <-time.After(uploadRetryBackoff):
				case <-ctx.Done():
					return
				}
			}
			continue
		}
		state.lastUploadedTime = time.Now()
		log(fmt.Sprintf("[%s] Uploaded successfully to '%s'.", path, branch))
		return
	}
}
