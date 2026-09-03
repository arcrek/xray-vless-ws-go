# OTA Self-Update (`--update`)

Status: implemented and reviewed (all 4 phases complete, 2026-09-03)
Created: 2026-09-03

## Summary

Add a manual, CLI-triggered self-update: `./xrayws --update` checks the
latest GitHub Release, downloads the matching platform asset, verifies its
SHA256 against the published `SHA256SUMS`, atomically replaces the running
binary in place, and restarts the `xrayws` systemd service. Also adds
`./xrayws --rollback` to swap back to the immediately-preceding binary
(single level, no history) and restart. No background auto-check, no
dashboard integration.

## Resolved open questions (user review, 2026-09-03)

1. **Rollback** — IN scope. `--update` preserves the pre-update binary as
   `<exec>.prev` (via hardlink, not copy — see Decision 4a); `--rollback`
   swaps it back and restarts. Single level only (no history chain).
2. **Windows** — clean refusal. `--update`/`--rollback` on
   `runtime.GOOS == "windows"` print "self-update while running isn't
   supported on Windows — download the new build manually" and exit
   non-zero; no swap attempted.
3. **Swap-succeeds/restart-fails** — exit 1 with an explicit "binary
   updated to vX.Y.Z, but automatic restart failed: `<err>` — restart
   manually: `sudo systemctl restart xrayws`" message. Non-zero exit even
   though the binary swap itself succeeded, so scripts checking the exit
   code see the failure.
4. **Trust model** — SHA256SUMS only, no GPG/signature verification.
   Matches `install.sh`'s existing model exactly; documented explicitly as
   integrity-against-corruption, not authenticity-against-compromise.

## Decided requirements (do not re-litigate)

- Trigger: manual CLI flag only (`--update`). No background auto-check, no
  dashboard button/API endpoint.
- Apply mechanism: self-replace the running binary on disk, then
  `systemctl restart xrayws` (the fixed unit name `install.sh` creates).
  No self-exec/no-systemd fallback needed for the *restart* step — restart
  is systemd-only. (Self-*replace*, i.e. the atomic binary swap, still must
  work correctly cross-platform per the non-goals section below — it's only
  the restart step that's systemd-only.)

## Background / current state (verified in this repo)

- `cmd/xrayws/main.go:30-32` — flags are declared inline with stdlib
  `flag` (`ciMode`, `logPort`), `flag.Parse()`, then dereferenced into
  `run(ctx, *ciMode, *logPort)`.
- No version string exists anywhere in the repo (`grep -r "Version"
  cmd/ internal/` — no hits). No `-ldflags -X` usage anywhere.
- `Makefile`: `LDFLAGS := -s -w` (line 3) is used by every `build-*`
  cross-compile target but **not** by the plain `build:` target (local dev
  build, line 10-11).
- `.github/workflows/build.yml`: resolves a release tag into
  `steps.tag.outputs.tag` (lines 41-49) for the GitHub Release itself, but
  never threads it into `make build-all` — the Go build has no idea what
  tag it's being built for.
- `install.sh` (the pattern to mirror, not reinvent):
  - Asset naming: `xrayws-<goos>-<goarch>` (lines 152-164), exactly
    matching `Makefile`'s `build-*` output names.
  - "latest" resolution: static redirect URL
    `https://github.com/$REPO/releases/latest/download/<asset>` (no REST
    API call). We deliberately diverge from this for `--update` (see
    Decision 2 below) because we need to *know* the resolved version, not
    just fetch a file.
  - Download + verify: downloads `<asset>` and `SHA256SUMS`, greps the
    matching line, computes actual sha256, hard-fails on mismatch (lines
    176-206).
  - systemd unit: fixed name `xrayws.service` (lines 285-321), no `User=`
    directive (inherits whoever started it — typically root, since
    `install.sh` itself sudo's the `systemctl` calls). Restart command is
    `sudo systemctl restart xrayws` (or plain `systemctl restart xrayws`
    if already root) — `install.sh` detects UID 0 via `do_sudo=""` vs
    `"sudo"` (lines 262-265).
- `.github/workflows/build.yml:39` generates `SHA256SUMS` via
  `sha256sum *` in `bin/` — flat `<hex-digest>  <filename>` lines, one per
  platform binary.
- `internal/ci/bridge.go:17-19` — existing precedent for a GitHub API
  client idiom: `var githubAPIBase = "https://api.github.com"` package var
  (overridable in tests), `Authorization: Bearer <token>` header pattern in
  `dispatchWorkflow`.
- `internal/cfdeploy/client.go:22-95` — existing precedent for this repo's
  REST-client shape: a small unexported `client` struct wrapping
  `*http.Client` + `baseURL` (+ auth token), a `doRequest` helper that
  unwraps a JSON envelope into `(json.RawMessage, error)`, and a
  package-level `var apiBaseURL = "..."` solely so tests can point it at an
  `httptest.Server`.
- `internal/tunnel/download.go:100-139` — downloads the cloudflared binary
  via plain `os.Create` + `io.Copy`, **not** atomically (no temp file, no
  rename). This is a *different, sibling process* being downloaded, so a
  half-written file there just fails the next launch attempt cleanly. This
  pattern must **not** be copied for self-replacement, where a half-written
  binary in place would brick the running service (see Decision 4).
- No self-replace/atomic-rename precedent exists anywhere in this repo
  (`grep -rn "os.Rename\|os.Executable"` — zero hits).
- `.env` / `config.Load()` requires a fully populated `.env` (UUID, ports,
  etc.) before the rest of `run()` can proceed — `--update` must not
  depend on this, since a broken/incomplete `.env` shouldn't block updating
  the binary that would otherwise fix things.

## Key decisions

### 1. Where `--update` branches in `main()`

`--update` gets its own top-level flag alongside `--ci-mode`/`--log-port`,
but branches **before** `config.Load()` and all proxy bootstrap (xray-core
engine, tunnel supervisor, log server). Rationale: update must work even
against a broken/missing `.env`, must not require any xray-core
initialization, and exits the process on completion (success or failure)
rather than falling through to `run()`'s normal server loop.

```go
func main() {
    ciMode := flag.Bool("ci-mode", false, "...")
    logPort := flag.Int("log-port", 9999, "...")
    doUpdate := flag.Bool("update", false, "Check GitHub Releases for a newer version, download+verify+install it, and restart the systemd service")
    doRollback := flag.Bool("rollback", false, "Swap back to the binary from before the last --update and restart the systemd service")
    showVersion := flag.Bool("version", false, "Print the running binary's version and exit")
    flag.Parse()

    if *showVersion {
        fmt.Println(Version)
        return
    }

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    if *doUpdate {
        if err := selfupdate.Run(ctx, Version, os.Stdout); err != nil {
            fmt.Fprintf(os.Stderr, "[!] update failed: %v\n", err)
            os.Exit(1)
        }
        return
    }

    if *doRollback {
        if err := selfupdate.Rollback(ctx, os.Stdout); err != nil {
            fmt.Fprintf(os.Stderr, "[!] rollback failed: %v\n", err)
            os.Exit(1)
        }
        return
    }

    if err := run(ctx, *ciMode, *logPort); err != nil { ... }
}
```

`selfupdate.Run` owns the entire flow (resolve latest → compare versions →
download → verify → swap → restart) and prints its own progress lines,
mirroring `install.sh`'s narrated-steps UX. It returns a plain `error`;
`main` reports and exits non-zero on failure, matching the existing
top-level error-handling idiom.

`--version`/`-v` ships alongside `--update` in the same change, because
without it there would be no way to inspect the running binary's version
at all — neither to decide whether an update is needed before running it,
nor to confirm afterward that `--update` actually moved the version
forward. This is a small, clearly-justified addition to scope, not scope
creep.

### 2. GitHub Releases lookup: REST API, not the static redirect URL

`install.sh` uses the static `.../releases/latest/download/<asset>`
redirect because it never needs to *know* which version it fetched. This
feature needs to know that (to report "updated v1.2.0 → v1.3.0" and to
support a same-version no-op short-circuit), so it uses:

```
GET https://api.github.com/repos/arcrek/xray-vless-ws-go/releases/latest
```

unauthenticated (public repo, no token required — same as `install.sh`'s
`wget` calls, which also send no auth). Following `internal/ci/bridge.go`'s
idiom, the base URL is a package var:

```go
var githubAPIBase = "https://api.github.com"
```

overridable in tests via an `httptest.Server`. The JSON response's
`tag_name` field becomes the "latest" version string; `assets[].name` +
`assets[].browser_download_url` resolve the platform-specific asset and
the `SHA256SUMS` asset by exact name match (`xrayws-<goos>-<goarch>` and
`SHA256SUMS`), same naming `install.sh` already relies on.

GitHub's unauthenticated REST API rate limit is 60 req/hour per IP — a
manually-triggered, human-paced flag is nowhere near that; no auth token
plumbing needed (also avoids requiring `GITHUB_TOKEN` to be present outside
CI, unlike `internal/ci`, which already assumes an Actions environment).

### 3. Version plumbing: `-ldflags -X`, `git describe` in `Makefile`, threaded through CI

- Add `var Version = "dev"` to `cmd/xrayws/main.go` (top-level, next to the
  existing package doc comment) — kept in `main` rather than a new
  micro-package to avoid introducing a package whose only content is one
  string var; `internal/selfupdate` takes it as a parameter
  (`selfupdate.Run(ctx, Version, ...)`) rather than importing `main`
  (imports never point at `main`).
- `Makefile`:
  ```make
  VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
  LDFLAGS := -s -w -X main.Version=$(VERSION)
  ```
  Applied to **both** the plain `build:` target (currently missing
  `-ldflags` entirely — needed so a locally-built binary can be used to
  exercise/test the update flow, e.g. comparing a `dev`-vs-real-tag
  mismatch or a `v1.2.0-3-gabc1234-dirty` string) and all `build-*`
  cross-compile targets (which already pass `$(LDFLAGS)`, so they pick this
  up for free once `LDFLAGS` includes the `-X` flag).
  `--always` falls back to a short commit hash when no tag exists yet in
  the checkout; `--dirty` appends `-dirty` when the working tree has
  uncommitted changes; final fallback `dev` covers a non-git checkout (e.g.
  a source tarball with no `.git/`), where `git describe` itself errors.
- `.github/workflows/build.yml`: thread the already-resolved
  `steps.tag.outputs.tag` into the build step so released binaries report
  their *actual* release tag (not a `git describe` guess, which could
  differ from the release tag on a `workflow_dispatch` rebuild of an
  existing tag). Change the "Cross-compile all platforms" step to pass
  `VERSION` explicitly:
  ```yaml
  - name: Resolve release tag
    id: tag
    ...
  - name: Cross-compile all platforms
    run: make build-all VERSION=${{ steps.tag.outputs.tag }}
  ```
  This requires moving the "Resolve release tag" step *before* "Cross-compile
  all platforms" in the job (currently it comes after — see current
  `build.yml` step order, lines ~32-49). `make build-all VERSION=...`
  works because `Makefile`'s `VERSION :=` is a simple recursively-expanded
  var; a `VERSION=x` argument on the `make` command line overrides it for
  that invocation (standard `make` variable-override semantics — no
  `Makefile` change needed beyond the `VERSION :=`/`LDFLAGS` addition
  above).

### 4. `internal/selfupdate` package: atomic self-replace

New package `internal/selfupdate/` (flat, one level under `internal/`,
matching `internal/cfdeploy`, `internal/tunnel`, `internal/ci`).

Files (mirroring `internal/cfdeploy`'s one-file-per-concern layout):

- `github.go` — `latestRelease(ctx, apiBase) (tag string, assets map[string]assetInfo, err error)`. Minimal client, no shared `client` struct needed (single GET, no auth) — a plain `http.Get` + JSON decode is enough; add a `client`-style wrapper only if a second endpoint gets added later (YAGNI, consistent with this repo's stated bias — see architecture.md's "Fixed constants" / "fixed, not configurable" notes elsewhere).
- `verify.go` — `downloadAndVerify(ctx, assetURL, sumsURL, assetName string, destTmp string) error`: downloads the asset to `destTmp` (already positioned in the executable's directory — see below), downloads `SHA256SUMS` into memory, parses the flat `sha256sum`-style lines, finds the line matching `assetName`, computes `sha256.Sum256` over the downloaded temp file's bytes (streamed, not loaded fully into memory — `io.Copy` into a `sha256.New()` writer alongside the file write, or a second pass with `io.Copy(hasher, file)` after closing), hard-fails on mismatch. Mirrors `install.sh` lines 176-206 exactly in behavior (same two-file-download, same grep-by-exact-name matching, same hard-fail-on-mismatch), just in Go.
- `swap.go` — `atomicReplace(execPath, tmpPath string) error`: preserves
  the current binary for rollback, then `chmod +x` the temp file, then
  `os.Rename(tmpPath, execPath)`. Both paths must be on the same
  filesystem for `os.Rename` to be atomic — guaranteed by writing the temp
  file into `filepath.Dir(execPath)` in the first place (e.g.
  `execPath + ".update-tmp"` or
  `os.CreateTemp(filepath.Dir(execPath), "xrayws-update-*")`), never
  `os.TempDir()` (which may be a different filesystem/tmpfs, e.g. `/tmp`
  vs `/opt/xrayws`). On Linux, renaming over a currently-executing binary
  is safe and standard — the running process keeps its old inode open via
  its existing file descriptor/mapping; the new inode only takes effect on
  the *next* exec. This is exactly why the restart step (`systemctl
  restart`) is required afterward — the old process keeps running the old
  code in memory until it's restarted.
  All failure paths (download error, checksum mismatch, chmod failure)
  return before any `os.Rename` call and leave only the `*.update-tmp`
  file touched — the original binary at `execPath` is never opened for
  writing directly, so a failed update always leaves the previous, working
  binary in place. Add a `defer os.Remove(tmpPath)` that's a no-op after a
  successful rename (rename already consumed the tmp path) — cleans up a
  leftover partial file on any early-return failure path too.

### 4a. Rollback design (`<exec>.prev`, single level)

`--update` preserves the pre-update binary before the atomic swap, via a
**hardlink**, not a full copy: `os.Link(execPath, execPath+".prev")`. This
creates a second directory entry pointing at the *same inode* as the
current binary — free (no data copy) and instant. Renaming the new binary
into `execPath` afterward only changes what `execPath` points to; the
`.prev` entry keeps pointing at the old inode's unchanged bytes. If a
`.prev` from an earlier update already exists, remove it first
(`os.Remove`, ignoring `ENOENT`) before linking — this repo's rollback
scope is explicitly single-level (the binary immediately before the *most
recent* update), not a history chain, so each `--update` run replaces
whatever `.prev` existed before it.

Sequence inside `atomicReplace` (called only after `downloadAndVerify` has
already confirmed the new binary is good):
1. `os.Remove(execPath + ".prev")` if it exists (best-effort, ignore
   not-exist).
2. `os.Link(execPath, execPath+".prev")` — hardlink, same filesystem
   (guaranteed, same directory).
3. `os.Chmod(tmpPath, 0755)`.
4. `os.Rename(tmpPath, execPath)` — atomic swap to the new binary.

If step 2 (the `Link`) fails (e.g. filesystem doesn't support hardlinks —
rare on Linux ext4/xfs, but possible on some overlay/network filesystems),
treat it as a hard failure and abort *before* step 4 — better to fail the
update cleanly than silently ship an update with no rollback safety net.
Surface this as a clear error rather than silently skipping rollback
preservation.

`--rollback` (`rollback.go`, `Rollback(ctx, out) error`): symmetric, much
simpler flow — no download/verify needed, the `.prev` file was already
verified when it was the *active* binary:
1. `os.Executable()` → `execPath`; `prevPath := execPath + ".prev"`.
2. If `prevPath` doesn't exist, fail with a clear
   `"no previous binary to roll back to (run --update at least once first)"`
   error — do not attempt a partial/best-effort rollback.
3. Write-permission preflight on `filepath.Dir(execPath)` (same check as
   `--update`, Decision 5).
4. `os.Rename(prevPath, execPath)` — atomic, single rename, swaps the
   `.prev` binary back into place. (This *consumes* `.prev` — after a
   rollback, `.prev` no longer exists; running `--rollback` twice in a row
   fails cleanly on the second attempt per step 2, matching the
   single-level-only scope.)
5. `restartService(ctx)` — same restart step and same partial-success
   error framing as `--update` (Decision 5/Resolved-Q3): if the rename
   succeeds but restart fails, report "rolled back to vX.Y.Z, but
   automatic restart failed: `<err>` — restart manually" and exit non-zero.

No separate version-tracking/marker is needed to distinguish a rolled-back
binary — it's the literal previous binary, so `--version` after a rollback
naturally reports the old version string (already embedded via `-ldflags
-X` at that binary's build time).
- `restart.go` — `restartService(ctx) error`: shells out to
  `systemctl restart xrayws` (fixed unit name, matching `install.sh`'s
  `SERVICE_NAME="xrayws"`, lines 259-260), prefixed with `sudo` if not
  already root (`os.Geteuid() != 0`, same check `install.sh` does with
  `id -u` at lines 262-265). Before attempting the restart, checks
  `systemctl is-enabled xrayws` (or `is-active`) to confirm the unit
  actually exists — if not, this means the binary isn't running under the
  systemd install path (e.g. manual foreground run, or `install.sh`'s
  no-systemd fallback), and `--update` should still have swapped the
  binary successfully but must print a clear manual-restart message
  instead of erroring — see Decision 6 (non-Linux/no-systemd behavior).
- `run.go` — `Run(ctx context.Context, currentVersion string, out io.Writer) error`: orchestrates the whole flow end-to-end, printing narrated progress lines to `out` (matching `install.sh`'s narration style, e.g. `"[*] Checking latest release..."`, `"[*] Current: v1.2.0, latest: v1.3.0"`, `"[*] Downloading xrayws-linux-amd64..."`, `"[+] Checksum verified"`, `"[+] Binary replaced"`, `"[*] Restarting xrayws.service..."`). Steps:
  1. `os.Executable()` to find the running binary's real path (resolve
     symlinks with `filepath.EvalSymlinks` — `os.Executable`'s own doc
     warns the result may be a symlink).
  2. GOOS/GOARCH → expected asset name (`xrayws-<runtime.GOOS>-<runtime.GOARCH>`).
  3. `latestRelease()` → tag + asset URLs.
  4. **Same-version short-circuit**: if `currentVersion == tag` (or
     `currentVersion == "dev"`, in which case there's no meaningful
     comparison — print a note and proceed anyway, since a `dev` build has
     no real version to compare against), print "already up to date" and
     return nil without downloading anything.
  5. **Write-permission preflight**: before downloading anything, verify
     the executable's directory is writable (attempt to create the temp
     file early, or an explicit `unix.Access`-style check) — fail fast
     with an actionable error ("permission denied writing to
     /opt/xrayws — try running with sudo") rather than downloading the
     full asset first and only then discovering the rename will fail (see
     Decision 5).
  6. `downloadAndVerify()`.
  7. `atomicReplace()`.
  8. `restartService()` — Linux+systemd only; see Decision 6 for
     non-Linux/no-systemd behavior.

### 5. Privilege/permission handling

- If the executable's directory isn't writable by the current user
  (typical case: binary installed by `install.sh` under a root-owned
  systemd deployment, `--update` invoked as a non-root user), fail with an
  explicit, actionable error **before** downloading anything:
  `"cannot write to <dir>: permission denied — re-run with sudo (the systemd deployment in install.sh normally runs as root)"`.
  Detected via a preflight write-access check (Decision 4, step 5), not by
  waiting for `os.Rename` to fail after a wasted download.
- If `systemctl restart` needs elevated privilege and the process isn't
  root, `restartService()` prefixes the command with `sudo` (matching
  `install.sh`'s own `do_sudo` pattern) — this only works non-interactively
  if the invoking user has passwordless sudo for `systemctl`, or if
  `--update` itself is simply run as root/via `sudo ./xrayws --update`
  (the expected common case, mirroring how `install.sh` itself is
  typically run). If the `sudo systemctl restart` invocation still fails
  (e.g. a password prompt that can't be satisfied non-interactively), the
  binary swap has **already succeeded** by this point — report that
  clearly: `"binary updated to vX.Y.Z, but automatic restart failed: <err> — restart manually: sudo systemctl restart xrayws"`.
  This is a partial-success, not a full failure — `selfupdate.Run` should
  still return a non-nil error (so `main` exits non-zero and the failure
  is visible), but the error message must make clear the update itself
  succeeded and only the restart step needs a follow-up.

### 6. Non-Linux / no-systemd behavior

- The atomic-replace step (download, verify, `os.Rename`) is plain Go
  standard library and works identically on Linux, macOS, and Windows —
  no reason to restrict it. **Windows caveat**: `os.Rename` cannot replace
  a file that's currently open/executing on Windows (unlike Linux/Unix) —
  renaming over the currently-running `.exe` will fail with a
  sharing-violation error. Given the decided scope is systemd-restart-only
  and this repo's install/production path is Linux+systemd, `--update` on
  Windows should detect this case (attempt the rename, catch the specific
  failure, or just detect `runtime.GOOS == "windows"` upfront) and either:
  (a) refuse cleanly with "self-update while running isn't supported on
  Windows — download the new build manually", or (b) download+verify into
  a sibling file (e.g. `xrayws.exe.new`) and print instructions to swap it
  in after closing the running process. Recommend (a) for v1 — simpler,
  avoids inventing a two-step manual dance nobody has asked for; can be
  revisited if a Windows user actually files this.
- On Linux/macOS without systemd (e.g. `install.sh`'s own no-systemd
  foreground-fallback path), the atomic swap still runs and succeeds, but
  `restartService()` detects no `xrayws.service` unit (via
  `systemctl is-enabled xrayws` failing, or `systemctl` binary missing
  entirely) and skips the restart with a clear message instead of erroring:
  `"binary updated to vX.Y.Z — no systemd unit detected, restart the process manually to run the new version"`. This is a clean success path
  (`Run` returns nil), not a failure — matching `install.sh`'s own
  systemd-detection-and-fallback idiom (lines 217-227).
- macOS: same as the Linux non-systemd case (no systemd on macOS at all) —
  swap succeeds, restart step always prints the manual-restart message.

### 7. Rollback — IN SCOPE (resolved with user, see summary at top)

User confirmed rollback should be included. Implemented cheaply via a
hardlink (not a full copy — zero extra disk cost until the old inode's
last link is removed), single-level only (see Decision 4a for the full
design). `.prev`'s own integrity story is a non-issue: it's never
downloaded or independently verified — it's a hardlink to bytes that were
already the verified, previously-running binary, so there's nothing new to
trust.

## Package layout

```
internal/selfupdate/
├── github.go        # latestRelease(): GitHub REST API client (unauthenticated GET)
├── github_test.go
├── verify.go         # downloadAndVerify(): asset + SHA256SUMS download, sha256 check
├── verify_test.go
├── swap.go            # atomicReplace(): .prev hardlink + temp-file-in-same-dir + os.Rename
├── swap_test.go
├── restart.go        # restartService(): systemctl restart xrayws (+ sudo, + no-unit detection)
├── restart_test.go
├── run.go             # Run(): end-to-end update orchestration + progress narration
├── run_test.go
├── rollback.go        # Rollback(): swap <exec>.prev back into place + restart
└── rollback_test.go
```

## Testing strategy

Follow `internal/cfdeploy`'s and `internal/ci`'s existing httptest-mocking
idiom (see `internal/cfdeploy/account_test.go`, `internal/cfdeploy/client_test.go`):

- `github_test.go`: spin up an `httptest.Server` serving a canned
  `releases/latest` JSON body (with `tag_name` + `assets[]`), point
  `githubAPIBase` (package var, same pattern as `internal/ci/bridge.go`'s
  `githubAPIBase`) at it, assert `latestRelease()` parses tag + resolves
  the right asset URL for a given GOOS/GOARCH.
- `verify_test.go`: `httptest.Server` serving a fake asset body + a fake
  `SHA256SUMS` body computed from that same fake body; assert
  `downloadAndVerify` succeeds when the hash matches, fails cleanly (no
  partial file left, or file left only at the `*.update-tmp` path) when
  either (a) the server returns a mismatched asset body, or (b) the
  `SHA256SUMS` body has no line for the requested asset name.
- `swap_test.go`: use `t.TempDir()` as a stand-in "executable directory" —
  create a fake "current binary" file, call `atomicReplace` with a
  temp-file path in the same dir, assert the target file's contents and
  mode (`+x`) after success; assert the *original* file is untouched when
  a forced failure occurs before the rename call (e.g. test `chmod`
  failure path directly, or test that `downloadAndVerify`'s own failure
  never even reaches `atomicReplace`). Never touches a real binary or the
  actual test binary's own executable path.
- `restart_test.go`: cannot realistically exercise real `systemctl` in
  CI/sandboxed test runs — test the command-construction logic in
  isolation (e.g. extract a small pure function like
  `buildRestartCommand(isRoot bool) []string` returning
  `["sudo","systemctl","restart","xrayws"]` vs
  `["systemctl","restart","xrayws"]`, unit-test that in isolation) and the
  no-unit-detected branch by injecting a fake "is unit present" checker
  function (dependency-injected, not hardcoded to shell out, so the test
  can supply a stub returning false). Do not attempt to spin up a real
  systemd unit in tests.
- `run_test.go`: integration-style test of `Run()` wiring the above
  together against a fake GitHub server + `t.TempDir()` executable path +
  an injected no-op/fake restart function — assert the full happy path
  (download → verify → swap → `.prev` hardlink created → "restart" called)
  and the same-version short-circuit (no download attempted when
  `currentVersion == tag`).
- `rollback_test.go`: `t.TempDir()` executable path with a fake "current"
  binary and a fake `.prev` file already in place (distinct contents);
  assert `Rollback()` swaps them (target now holds the old `.prev`
  contents, `.prev` no longer exists) and calls the injected restart
  function; assert a clean, specific error (not a generic file-not-found)
  when no `.prev` file exists.

`restartService`'s systemctl-invocation should be structured as an
injectable function field/interface (similar to how `tunnel.Supervisor`
takes `LogLine`/`OnHostname`/`OnReady` as func fields) so `run_test.go` can
substitute a fake instead of actually shelling out — e.g.
`type Runner struct { ...; execCommand func(name string, args ...string) *exec.Cmd }`
defaulting to `exec.Command` in production, overridable in tests.

## Docs updates

- `README.md`: add a `--update`/`--rollback` section near the existing
  "Run (manual)" / "System dashboard" sections, following the same style
  (fenced command example + a short paragraph). Cover: what each does,
  that both require systemd for the auto-restart (with the no-systemd
  manual-restart fallback noted), single-level-only rollback semantics,
  the `--version` flag, and the permission note (may need `sudo` depending
  on install ownership). Also add `--version` to the fenced command list.
- `docs/architecture.md`:
  - New row in the Components table: `internal/selfupdate` — "GitHub
    Releases lookup, download+verify, atomic binary swap + single-level
    rollback, systemd restart for `--update`/`--rollback`".
  - New package listed in the tree diagram alongside `ci/`, `cfdeploy/`.
  - New "Decision log — OTA self-update (`internal/selfupdate`)" section,
    same table format as the cfdeploy decision logs, covering: trigger
    (manual flag only), version source (`-ldflags -X`, `git describe`),
    GitHub lookup method (REST API vs install.sh's static redirect, and
    why), atomic-swap mechanism, rollback mechanism (`.prev` hardlink,
    single-level), restart mechanism (fixed systemd unit name),
    privilege-failure handling, non-Linux/no-systemd behavior, and the
    SHA256SUMS-only trust model.

## Non-goals / out of scope (YAGNI)

- No background auto-check / scheduled polling for new releases.
- No dashboard button or `/update` HTTP API endpoint (`--log-port`'s
  `internal/logserver` stays untouched).
- No multi-level rollback history (only the single most-recent `.prev` is
  kept — no `xrayws.prev.1`, `.prev.2`, etc.).
- No Windows in-place self-restart handling — swap either refuses cleanly
  or (if chosen instead) writes a sibling `.new` file with manual-swap
  instructions; no attempt to kill-and-relaunch the running Windows
  process automatically.
- No signature/GPG verification beyond the existing SHA256SUMS check —
  matches `install.sh`'s current trust model exactly (SHA256 sourced from
  the same GitHub Release the binary comes from — this is integrity
  verification against transport corruption, not authenticity/tamper
  verification against a compromised release; call this out explicitly in
  docs rather than silently implying more security than it provides,
  since it's identical to what `install.sh` already does today).
- No changes to `install.sh` itself in this plan — `install.sh` remains
  the *initial*-install path; `--update` is a separate, purely-in-binary
  *subsequent*-update path. (Worth a one-line README cross-reference so
  users don't wonder why there are two mechanisms.)
- No support for downgrading to an older/specific version (e.g. no
  `--update=v1.2.0` pinning flag) — always moves to whatever `latest`
  currently resolves to. Simple flag-name reservation only if this becomes
  wanted later.

## Phase breakdown

### Phase 1 — Version plumbing
**Status: done** — `Version` var + `--version`/`-v` flags implemented in
`cmd/xrayws/main.go`; `Makefile` `VERSION`/`LDFLAGS` (git describe based)
applied to both `build` and `build-*` targets. Verified: `make build &&
./bin/xrayws --version` prints a real git-describe string.
- Add `var Version = "dev"` to `cmd/xrayws/main.go`.
- Add `--version`/`-v` flag, print-and-exit before any other branch.
- `Makefile`: add `VERSION :=` (via `git describe --tags --always --dirty`,
  fallback `dev`), extend `LDFLAGS` with `-X main.Version=$(VERSION)`,
  apply to `build:` (currently missing `-ldflags` entirely) as well as all
  existing `build-*` targets.
- Verify locally: `make build && ./bin/xrayws --version` prints a real
  `git describe` string; a fresh non-git-tracked copy of the source falls
  back to `dev`.
- **Review checkpoint**: confirm version-string format looks right before
  building the update-comparison logic against it in Phase 2.

### Phase 2 — `internal/selfupdate` package + tests
**Status: done** — package implemented (github.go, verify.go, swap.go,
restart.go, run.go, rollback.go + full `_test.go` coverage, including
integration-style tests of `Run()`/`Rollback()` via an injectable
`osExecutable` var). `go test ./internal/selfupdate/...` green (21 tests).
Review pass fixes applied: `isUnitPresent` now checks systemd `LoadState`
(via `systemctl show -p LoadState`) instead of `is-enabled`; shared
`httpClient` with a 5-minute timeout added (github.go/verify.go) instead of
`http.DefaultClient` with no bound; redundant `checkWritable` preflight in
run.go removed in favor of surfacing the real `os.CreateTemp` error wrapped
with `%w`.
- `github.go` + `github_test.go`: GitHub Releases API client (latest-tag +
  asset-URL resolution for current GOOS/GOARCH).
- `verify.go` + `verify_test.go`: download-to-temp + SHA256SUMS parse +
  verify.
- `swap.go` + `swap_test.go`: atomic same-directory rename, chmod +x,
  no-touch-on-failure guarantee.
- `restart.go` + `restart_test.go`: systemctl restart invocation
  (injectable exec function), sudo-prefix logic, no-unit-detected
  clean-skip branch.
- `run.go` + `run_test.go`: end-to-end orchestration, same-version
  short-circuit, write-permission preflight, `.prev` hardlink creation,
  narrated progress output.
- `rollback.go` + `rollback_test.go`: `.prev` existence check, atomic
  rename-back, restart, clean "nothing to roll back to" error.
- `go test ./internal/selfupdate/...` green, no real network/systemd calls
  in the test suite.
- **Review checkpoint**: walk through the failure-path guarantees (partial
  download, checksum mismatch, permission denied, restart failure after
  successful swap) before wiring this into `main.go` — these are the
  highest-consequence bugs (a broken `--update` could brick a running
  proxy), worth a deliberate pause here.

### Phase 3 — `--update`/`--rollback` flag wiring in `main.go`
**Status: done** — `--update`/`--rollback` flags wired into main.go,
branching before `config.Load()`/proxy bootstrap, per Decision 1. `-v`
shorthand added alongside `--version`.
- Add `--update` and `--rollback` flags, both branching before
  `config.Load()`/proxy bootstrap (Decision 1).
- Wire `selfupdate.Run(ctx, Version, os.Stdout)` and
  `selfupdate.Rollback(ctx, os.Stdout)`, error handling matching the
  existing top-level `os.Exit(1)` pattern.
- Manual smoke test against a real (or test) GitHub Release: run
  `--version`, run `--update` against an older locally-built binary,
  confirm swap + restart (or manual-restart message on a non-systemd dev
  box) behave as designed, then run `--rollback` and confirm it swaps back
  and the version reported by `--version` reverts.
- **Review checkpoint**: this is the first point real GitHub Releases
  infrastructure is exercised — confirm asset naming / SHA256SUMS format
  from an actual `build.yml` run matches what Phase 2 expects before
  calling this done.

### Phase 4 — CI wiring + docs
**Status: done** — `.github/workflows/build.yml` reordered (resolve tag
before cross-compile, `VERSION` passed through via an `env:` var + `"$VERSION"`
rather than direct `${{ }}` interpolation, hardening against script
injection); README.md and docs/architecture.md updated with a decision-log
section and component-table entry.
- `.github/workflows/build.yml`: reorder "Resolve release tag" before
  "Cross-compile all platforms", pass `VERSION=${{ steps.tag.outputs.tag }}`
  into `make build-all`.
- `README.md`: `--update`/`--version` sections.
- `docs/architecture.md`: Components table row, package-tree entry, new
  decision-log section.
- Tag a test release (or dry-run `workflow_dispatch`) to confirm the
  published binaries' `--version` output matches the release tag
  end-to-end.
- **Review checkpoint**: final pass before considering this fully done —
  confirm the README/architecture docs read correctly against the actually
  -implemented behavior (not just the plan).

All open questions were resolved with the user before implementation — see
"Resolved open questions" near the top of this document.

**All 4 phases implemented and reviewed (2026-09-03).** Two review passes
(tester + code-reviewer subagents) ran across the phases above; all
actionable findings were fixed (see per-phase status notes). Final
verification: `go test ./... -count=1` — 124 passed, 11 packages; `go build
./...`, `go vet ./...`, and `gofmt -l` all clean.

## Critical files for implementation

- `cmd/xrayws/main.go`
- `Makefile`
- `.github/workflows/build.yml`
- `internal/cfdeploy/client.go` (idiom reference)
- `install.sh` (behavior to mirror)
