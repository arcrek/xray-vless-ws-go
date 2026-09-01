# xray-vless-ws-go

Single-binary Go server for VLESS-WS over Cloudflare Tunnel, DPI-bypass via
Anycast/SNI decoupling.

## Architecture highlights

See [`docs/architecture.md`](docs/architecture.md) for the full component
breakdown and decision log.

- `xray-core` embedded as a Go library (in-process, no binary download, no
  subprocess, no `config.json` file round-trip).
- `cloudflared` kept as a managed subprocess (no stable embeddable API
  exists for it). The binary is downloaded automatically on first run.
- WARP outbound dropped from v1 (adds a WireGuard hop, against the
  "optimize for speed" goal).
- `.env` config format, kept simple and CI-secret friendly (works as a
  single `ENV_CONFIG` GitHub Actions secret — see the CI bridge section
  below).
- `/ready`-based tunnel health polling replaces blind `time.sleep(1)`
  restart loops.
- Graceful shutdown on both SIGINT and SIGTERM.

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
run directly under Termux either (Termux users are pointed at
`pkg install cloudflared` instead), so this is not a regression for the
realistic Android/Termux use case.

## Quickstart (install script)

One-liner — downloads `install.sh` and runs it straight away:

```
f=$(mktemp) && wget -qO "$f" https://raw.githubusercontent.com/arcrek/xray-vless-ws-go/main/install.sh && bash "$f"; rm -f "$f"
```

Or clone first and run it locally:

```
./install.sh
```

Prompts for the `.env` values (Enter keeps the default), downloads the
matching prebuilt binary + `SHA256SUMS` from this repo's latest GitHub
Release (plain unauthenticated `wget` against the public release URLs — no
token needed), and verifies the checksum. On Linux with systemd, the script
then smoke-tests the binary (TCP probe on the configured `PORT` for up to
15s) and, if healthy, creates and enables a `xrayws.service` systemd unit
that auto-restarts on failure. Without systemd (macOS, containers) or if the
smoke test fails, falls back to running in the foreground. See
`install.sh`'s header comment for env-var overrides (`INSTALL_DIR`,
`RELEASE_TAG`).

## Run (manual)

```
./bin/xrayws                    # foreground, quick tunnel or named tunnel per .env
./bin/xrayws --log-port=0       # disable the embedded log viewer
./bin/xrayws --ci-mode          # GitHub Actions CI bridge mode — see note below
```

On first run, a default `.env` is generated (see `.env.example` for the
full field list). Set
`TUNNEL_TOKEN` + `WS_HOST` for a named (production) tunnel, or leave both
empty for a zero-setup quick tunnel (dev/test only — see
`docs/architecture.md`'s decision log #7).

### System dashboard (`--log-port`, default 9999)

**Breaking change:** as of the system-dashboard feature, `LOG_PASSWORD` is
now **required** whenever the dashboard is enabled (`--log-port` > 0, which
is the default). The dashboard now serves live traffic stats and the
current tunnel hostname at `GET /stats`, not just log text — an
unauthenticated dashboard would expose meaningfully more than before, so
the binary now fails fast (before any listener opens) if `--log-port > 0`
and `LOG_PASSWORD` is empty. Set `LOG_PASSWORD` in `.env`, or pass
`--log-port=0` to disable the dashboard/log viewer entirely (still valid
with no password).

The dashboard at `http://<host>:<log-port>/` shows xray/tunnel status,
aggregate uplink/downlink throughput (a 5-minute sparkline), and the
existing realtime log pane, polling a JSON `/stats` endpoint every 2s
(same Basic Auth as `/logs`).

**Recommendation:** the server is plain HTTP with no TLS (Basic Auth
credentials travel in cleartext) — bind the dashboard port behind a
VPN/loopback-only setup rather than exposing it on a public interface.

`--ci-mode` (running the proxy itself inside a GitHub Actions runner via the
`ENV_CONFIG` secret) still exists in the code, but `.github/workflows/`
no longer invokes it — the Action now only builds and publishes release
binaries (see below). Run `--ci-mode` manually if you still want that
free-runner-as-host deployment.

## Cloudflare Worker auto-deploy (`internal/cfdeploy`)

Optional: set both of these in `.env` (see `.env.example`, or answer the
prompts in `./install.sh`) to have the binary provision the Worker bridge
(deploy the Worker script + bind Workers KV to it) via the Cloudflare REST
API on every startup, instead of doing it by hand in the dashboard.
Live-tested end-to-end 2026-08-29 against a real account — see
`docs/architecture.md`'s live-test notes for details, including one known
first-attach race (a `500` on the webhook that follows a *brand-new*
custom-domain attach, self-heals on the next run):

| Key | Meaning |
|---|---|
| `CLOUDFLARE_API_TOKEN` | Scoped API token — needs **Workers Scripts:Edit**, **Workers KV Storage:Edit**, **Workers Routes:Edit** (Zone), and **Zone:Read** (Zone) on `DOMAIN`'s zone. Also needs **Cloudflare Tunnel:Edit** (Account) and **DNS:Edit** (Zone) if you want the named-Tunnel auto-provision below. |
| `DOMAIN` | A zone apex already on the same Cloudflare account (e.g. `example.com` — **not** `vless.example.com`; the app prepends the `vless.` prefix itself, so the deployed hostname becomes `vless.example.com`). |
| `CLOUDFLARE_ACCOUNT_ID` | Optional — only needed if the token can see more than one account (auto-resolved via `GET /accounts` otherwise). |
| `WORKER_PASSWORD` | Generated once on first run (same idiom as `XRAY_UUID`) — becomes the deployed Worker's `/setapi?password=` check. |

Both `CLOUDFLARE_API_TOKEN` and `DOMAIN` empty (the default) skips this
entirely — `WEBHOOK_URL` is then used exactly as set in `.env`, unchanged
behavior. When active, the script is **re-deployed every run** (keeps the
deployed code current); the Workers KV namespace is **created once and
reused by name** on later runs, so stored config survives restarts. A
provisioning failure (bad token, zone not found, etc.) is logged as a
warning and never blocks startup — it just falls back to `WEBHOOK_URL` from
`.env`.

### Named Tunnel auto-provision

With `CLOUDFLARE_API_TOKEN` + `DOMAIN` set above, also leave `TUNNEL_TOKEN`
blank and set `WS_HOST` to a real hostname (a subdomain of `DOMAIN` other
than `vless.DOMAIN`, which the Worker route above reserves — e.g.
`tunnel.example.com`) to skip `cloudflared tunnel login/create/route dns`
entirely: the app finds-or-creates a named Tunnel (fixed name
`xray-vless-ws-bridge-tunnel`), points a proxied CNAME `WS_HOST` →
`<tunnel-id>.cfargotunnel.com` at it, and fetches a fresh connector token on
every start (never written to `.env` — Cloudflare tokens are re-fetchable on
demand, so there's nothing to keep in sync). A manually-set `TUNNEL_TOKEN`
is never overridden; leaving `WS_HOST` at the `trycloudflare.com` default
skips this and uses a quick tunnel as before.

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
meant to deliver: one in-process Go call replaces a
binary-download-check + config-file-write + separate-process cold start.

## CI bridge (`--ci-mode`)

Exports the `ENV_CONFIG` GitHub Secret to `.env`, watches
`frp_info.config`/`frp_info.json` for changes and
force-pushes them to the `config` branch, optionally self re-dispatches the
workflow after 5h (`BRIDGE_WORKFLOWS=true`), and self-exits after 5h40 to
stay under the Actions 6h job limit. No longer wired into
`.github/workflows/` — see the note in Run (manual) above.

## Build & Release Action (`.github/workflows/build.yml`)

Triggered by pushing a `v*` tag (or manually via `workflow_dispatch`):
cross-compiles all platforms (`make build-all`), generates `SHA256SUMS`,
and publishes them to a GitHub Release. `install.sh` consumes that Release.
This replaces the old `run.yml`, which used to run the proxy itself inside
the Actions runner.

Shells out to the `git` CLI for the branch push (not `go-git`) — see
`docs/architecture.md`'s decision log #11 for the tradeoff (this is the one
place the binary isn't fully self-contained; the Actions runner already has
`git`, so this costs nothing in CI).
