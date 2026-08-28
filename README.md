# xray-vless-ws-go

Production Go rebuild of [xray_vless_ws_server](https://github.com/vincentng295/xray_vless_ws_server)
(Python PoC for VLESS-WS over Cloudflare Tunnel, DPI-bypass via Anycast/SNI decoupling).

## Key differences from the Python PoC
- `xray-core` embedded as a Go library (in-process, no binary download, no
  subprocess, no `config.json` file round-trip).
- `cloudflared` kept as a managed subprocess (no stable embeddable API
  exists for it — see `plans/260828-1928-xray-go-rebuild/phase-03-tunnel-manager.md`).
  The binary is downloaded automatically on first run.
- WARP outbound dropped from v1 (adds a WireGuard hop, against the
  "optimize for speed" goal — see the plan's decision log #5).
- `.env` config format kept for backward compatibility with the Python
  version's `ENV_CONFIG` GitHub secret.
- `/ready`-based tunnel health polling replaces blind `time.sleep(1)`
  restart loops.
- Graceful shutdown on both SIGINT and SIGTERM (the Python version only
  handled SIGINT/KeyboardInterrupt).

## Build

```
make build              # local dev build -> bin/xrayws
make build-all          # cross-compile: linux/amd64, linux/arm64,
                         # windows/amd64, darwin/amd64, darwin/arm64
make build-android-arm64 # see "Known limitation" below
make test
make vet
```

All cross-compile targets except `android/arm64` build cleanly with
`CGO_ENABLED=0` — a fully static binary, no libc dependency, trivial to ship.

### Known limitation: android/arm64

`GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build` currently fails at link
time:

```
link: github.com/wlynxg/anet: invalid reference to net.zoneCache
```

This comes from a transitive dependency
(`xray-core → infra/conf → transport/internet/finalmask/realm →
github.com/pion/stun/v3 → github.com/pion/transport/v4/stdnet →
github.com/wlynxg/anet`) that isn't reachable through anything this
project's own inbound/outbound config uses (VLESS inbound + freedom
outbound only) — it's pulled in unconditionally because `infra/conf`
compiles the shape for xray-core's *entire* config schema, not just the
subset this project builds. `wlynxg/anet` uses a `go:linkname` reference
into the Go runtime's internal `net` package that doesn't resolve against
this project's pinned Go toolchain (1.26) on `GOOS=android`. This is an
upstream compatibility gap, not something fixable in this repo's own code;
tracked as a known limitation rather than silently dropped from the
supported-platforms list. Note that `cloudflared` itself already doesn't
run directly under Termux either (the Python PoC's own downloader flags
this — Termux users are pointed at `pkg install cloudflared` instead), so
this is not a regression for the realistic Android/Termux use case.

## Run

```
./bin/xrayws                    # foreground, quick tunnel or named tunnel per .env
./bin/xrayws --log-port=0       # disable the embedded log viewer
./bin/xrayws --ci-mode          # GitHub Actions CI bridge mode (see below)
```

On first run, a default `.env` is generated (same defaults as the Python
PoC's `.env.example`, minus the removed `ENABLE_WARP` key). Set
`TUNNEL_TOKEN` + `WS_HOST` for a named (production) tunnel, or leave both
empty for a zero-setup quick tunnel (dev/test only — see the plan's decision
log #7).

## Startup latency

Measured on this development machine, `bin/xrayws-linux-amd64` (stripped,
`CGO_ENABLED=0`), cloudflared binary already cached locally (no download):

| Milestone | Elapsed |
|---|---|
| Process start → log viewer HTTP server up | ~18ms |
| → xray-core VLESS+WS inbound listening and ready for traffic | ~35ms |
| → cloudflared subprocess launched | ~38ms |

The embedded xray-core engine has zero subprocess-spawn or `config.json`
file-round-trip overhead — this is the concrete "faster" win embedding was
meant to deliver. A literal side-by-side timing against a Python PoC run
was not captured in the same session (this repo has no Python environment
set up here); the qualitative difference — one in-process Go call vs.
Python's binary-download-check + JSON-file-write + `subprocess.Popen` +
xray's own separate-process cold start — is the basis for the "faster"
claim, backed by the concrete embedded-engine number above.

## CI bridge (`--ci-mode`)

Mirrors `github-workflows.py`: exports the `ENV_CONFIG` GitHub Secret to
`.env`, watches `frp_info.config`/`frp_info.json` for changes and
force-pushes them to the `config` branch, optionally self re-dispatches the
workflow after 5h (`BRIDGE_WORKFLOWS=true`), and self-exits after 5h40 to
stay under the Actions 6h job limit. See `.github/workflows/run.yml`.

Shells out to the `git` CLI for the branch push (not `go-git`) — see the
plan's decision log #11 for the tradeoff (this is the one place the binary
isn't fully self-contained; the Actions runner already has `git`, so this
costs nothing in CI).

## Cutover status

The original Python version (`../xray_vless_ws_server/`) is untouched and
still fully functional. Per the plan's Phase 7, Python file removal is a
separate, explicit follow-up decision — not bundled with this rewrite —
once the Go version has run successfully in the real GitHub Actions
workflow at least once.
