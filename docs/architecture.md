# Architecture

Single-binary Go server for VLESS-WS over Cloudflare Tunnel. This document
consolidates the design decisions and rationale behind the codebase — the
canonical reference for "why is it built this way", replacing the local
planning notes that produced it (never shipped in this repo).

## Components

| Package | Role |
|---|---|
| `internal/config` | `.env` loading (via `godotenv`), validation, defaults, secret generation |
| `internal/xraycore` | Embedded xray-core lifecycle + JSON config builder |
| `internal/tunnel` | cloudflared download, spawn, hostname/ready detection, restart supervision |
| `internal/linkgen` | `vless://` link building, `frp_info.config`/`.json` writer, webhook delivery |
| `internal/logserver` | Embedded HTTP dashboard: realtime log viewer + `/stats` (xray/tunnel status, traffic throughput) — `go:embed` assets from `web/logserver` |
| `internal/ci` | GitHub Actions CI bridge (export secret, watch+upload, re-dispatch) |
| `internal/cfdeploy` | Cloudflare Worker bridge + named Tunnel auto-provision via the Cloudflare REST API |

```
cmd/
├── xrayws/               # entrypoint, flag parsing (--ci-mode etc.)
└── xraycore-smoketest/   # standalone embedding smoke test
internal/
├── config/
├── xraycore/
├── tunnel/
├── linkgen/
├── logserver/
├── ci/
└── cfdeploy/
web/
└── logserver/            # static HTML/CSS/JS for the log viewer (go:embed source)
```

## Decision log — core rebuild

| # | Decision | Chosen | Rejected alternatives |
|---|----------|--------|------------------------|
| 1 | xray-core integration | Embed as Go library (`core.StartInstance`) | Spawn a downloaded binary (loses single-binary + startup-speed win) |
| 2 | cloudflared integration | Keep as subprocess, download binary in Go | Embed as library (no stable Go API for external embedding) |
| 3 | Quick-tunnel hostname discovery | Parse stdout for the "Your quick Tunnel has been created!" line (no `/quicktunnel` metrics endpoint exists) | Metrics `/quicktunnel` JSON endpoint (does not exist) |
| 4 | Tunnel health/restart detection | Poll the `--metrics` server's `/ready` endpoint (200/503 + `readyConnections`) | Only process-exit detection (loses "connected but stalled" case) |
| 5 | WARP outbound | Out of scope for v1 | wgcf-cli subprocess, native Go WireGuard client — both add an extra WireGuard hop, against the "optimize for speed" goal |
| 6 | Config format | `.env`, parsed with `godotenv` | YAML/TOML (breaks compatibility with an existing `ENV_CONFIG` GitHub secret deployment) |
| 7 | Named tunnel vs quick tunnel | Named tunnel (`TUNNEL_TOKEN` + `WS_HOST`) is the recommended production path — hostname is known upfront, zero log-parsing needed. Quick tunnel is dev/test only. | — |
| 8 | Concurrency model | Goroutines + channels + `context.Context` | A raw thread + fixed-interval sleep poll loop |
| 9 | Cloudflare Worker bridge (`internal/cfdeploy`) | Kept as an optional companion component; later automated via the Cloudflare REST API (see the cfdeploy decision log below) | — |
| 10 | Repo location | Standalone repo (private), self-contained | Nested inside another repo |
| 11 | CI bridge git integration | `git` CLI via `os/exec` for the branch push (accepts the "not a pure single binary for `--ci-mode`" tradeoff) | `go-git` (pure Go, rejected: bigger dependency, force-push/orphan-branch edge-case parity risk) |
| 12 | `hidden/`-prefix branch auto-delete logic | Dropped — confirmed dead code (no caller ever uses a `hidden/`-prefixed branch) | Implement for parity with a prior design (rejected: YAGNI) |

## Decision log — Cloudflare Worker auto-deploy (`internal/cfdeploy`)

| # | Decision | Chosen | Rejected alternatives |
|---|----------|--------|------------------------|
| 1 | Auth | `CLOUDFLARE_API_TOKEN` (Bearer), scoped token only | Global API Key + Email (legacy, broader blast radius) |
| 2 | Account ID | Auto-resolve via `GET /accounts`, use first result; warn + suggest pinning if >1 account visible to the token. `CLOUDFLARE_ACCOUNT_ID` env is an optional override. | Force the user to always supply it |
| 3 | Hostname | Custom domain route: fixed prefix `vless.` + `DOMAIN` (e.g. `vless.example.com`), via `PUT /accounts/{id}/workers/domains` with `zone_name=DOMAIN` | `*.workers.dev` default subdomain |
| 4 | Worker password | `WORKER_PASSWORD` env field; random-generated fallback if missing, same idiom as `XRAY_UUID` | Reuse `LOG_PASSWORD` |
| 5 | Idempotency | Script content is **re-uploaded every run** (keeps deployed code current); KV namespace is **created once**, found-and-reused by title on later runs (config in KV survives restarts) | Skip entirely if already deployed |
| 6 | Scope of automation | Only Worker script deploy + Workers KV binding. WAF/DNS-proxy toggles and zone creation are out of scope. | Also automate zone onboarding |
| 7 | Worker script source | The binary embeds and deploys its own copy (`internal/cfdeploy/assets/worker.js`) | Read/deploy an externally-hosted copy at runtime (adds a runtime dependency) |
| 8 | Failure handling | Any provisioning error is logged as a warning and is non-fatal — app continues, `WebhookURL` falls back to whatever static `WEBHOOK_URL` is already in `.env` | Crash/exit on provisioning failure |

### cfdeploy architecture

New package `internal/cfdeploy`, zero import from `internal/tunnel`/`internal/linkgen` (depends only on `internal/config`):

- `client.go` — minimal REST client: `http.Client` + `baseURL` + Bearer token; unwraps Cloudflare's `{success, errors[], result}` envelope into a Go error on `success:false` or non-2xx.
- `account.go` — `ResolveAccountID`: returns an override verbatim if set; else `GET /accounts`, errors if empty, logs+uses-first if more than one.
- `kv.go` — `EnsureKVNamespace`: `GET .../storage/kv/namespaces`, matches by title; else `POST` creates it. No pagination on the list call — acceptable at personal-account scale.
- `script.go` — `UploadScript`: builds the multipart body (`metadata` JSON + `worker.js` source, `Content-Type: application/javascript+module`), `PUT /accounts/{id}/workers/scripts/{name}`.
- `domain.go` — `AttachCustomDomain`: `PUT /accounts/{id}/workers/domains` with `{hostname, service, zone_name}`.
- `deploy.go` — `Ensure(ctx, cfg) (webhookURL string, err error)`: no-op when `CloudflareAPIToken`/`Domain` are empty; else runs Resolve → EnsureKVNamespace → UploadScript → AttachCustomDomain and returns `https://<hostname>/setapi?password=<WorkerPassword>`.
- `assets/worker.js` (`go:embed`) — the Worker source, with the password line's literal replaced by a marker `__WORKER_PASSWORD__` before upload (substitution goes through `json.Marshal` so a password containing `"` or `\` can't break out of the deployed script's string literal).
- `tunnel.go` — `ensureNamedTunnel`: find-or-create a named Tunnel by fixed name, fetch a fresh connector token, resolve `DOMAIN`'s zone, and find-or-create/patch a proxied CNAME `WS_HOST` → `<tunnel-id>.cfargotunnel.com`. No-op unless `TUNNEL_TOKEN` is blank and `WS_HOST` names a real (non-default, non-Worker-hostname) subdomain of `DOMAIN`.

Fixed constants (not env-configurable, YAGNI): script name `xray-vless-ws-bridge`, KV namespace title `xray-vless-ws-bridge-kv`, tunnel name `xray-vless-ws-bridge-tunnel`, hostname prefix `vless`, `compatibility_date` pinned to a fixed string.

## Decision log — Cloudflare named Tunnel auto-provision (`internal/cfdeploy/tunnel.go`)

| # | Decision | Chosen | Rejected alternatives |
|---|----------|--------|------------------------|
| 1 | Trigger | Reuse existing fields, no new env vars: fires when `CLOUDFLARE_API_TOKEN`+`DOMAIN` are set, `TUNNEL_TOKEN` is blank, and `WS_HOST` is a real hostname (not the quick-tunnel default) | A dedicated `AUTO_TUNNEL=true` flag |
| 2 | Token persistence | Never written to `.env` — fetched fresh from `GET .../cfd_tunnel/{id}/token` on every run, same as `cloudflared tunnel token` is re-runnable any time | Write back to `.env` like a generated password (adds a write-back code path this package didn't have) |
| 3 | Idempotency | Tunnel found-or-created by fixed name (`xray-vless-ws-bridge-tunnel`, list-then-create, same idiom as `ensureKVNamespace`); DNS CNAME found-and-left-alone if already correct, patched if stale, created if absent | Always delete+recreate |
| 4 | Config source | `config_src: cloudflare` (remotely-managed) tunnel at creation, but ingress is still driven by the **local** `config.yml` `internal/tunnel` writes from `WS_HOST` — no remote ingress API call needed | Push ingress rules via `PUT .../cfd_tunnel/{id}/configurations` too (would duplicate `WriteNamedTunnelConfig`'s job) |
| 5 | Hostname collision guard | Refuse (log + no-op, not fatal) when `WS_HOST` equals the Worker's own `vless.DOMAIN` hostname — that hostname's DNS is already claimed by the Workers custom-domain route | Let the DNS record create call fail on its own |
| 6 | Zone scope | Only ever resolves `DOMAIN`'s own zone (`GET /zones?name=DOMAIN`) — `WS_HOST` must be `DOMAIN` or a subdomain of it | Resolve any zone the token can see (would need listing all zones + suffix-matching) |

## Live-test notes (2026-08-29)

- **cfdeploy, live-tested end-to-end** against a real Cloudflare account: KV
  namespace create, Worker multipart script upload, and custom domain
  attach all succeeded on the first run; a second run correctly reused the
  existing KV namespace by title and re-uploaded the script. Full chain
  verified with a real VLESS client — `client → vless.<DOMAIN> (Worker) →
  quick tunnel → embedded xray-core → internet` — outbound TCP connections
  opened and data flowed both directions.
- **Known race**: the webhook POST that follows a custom-domain attach in
  the same run can hit the domain within the propagation window and get a
  `500` from Cloudflare's edge (route not live yet) — non-fatal per
  decision #8 above, `WebhookURL` just falls back to `.env`. A second run
  against an already-attached domain sends the webhook successfully; no
  retry/backoff was added since the non-fatal fallback already absorbs it
  and the next run self-heals.

## Live-test notes (2026-09-01) — named Tunnel auto-provision

- **Bug found in first real run**: `internal/tunnel`'s `WriteNamedTunnelConfig`
  put `TUNNEL_TOKEN` into config.yml's `tunnel:` field. That field means a
  tunnel UUID/name for the legacy local-credentials (`cert.pem`) flow, not
  a token — cloudflared tried to resolve it as an ID, found no `cert.pem`,
  and crash-looped: `error parsing tunnel ID: Error locating origin cert`.
  This path was never live-tested with a real token before (the only prior
  named-tunnel exercise was a manually-typed, invalid token that failed
  earlier, at token *parsing*, never reaching this code). Fixed by dropping
  `tunnel:` from config.yml entirely and passing the token via cloudflared's
  `--token` flag instead (`internal/tunnel/launch.go`) — config.yml now
  carries ingress rules only.

## `.env` reference

See `.env.example` for the full annotated template. Summary:

| Key | Default | Notes |
|---|---|---|
| `PORT` | `127.0.0.1:8888` | Comma-separated `host:port` list for xray-core's VLESS inbound(s) |
| `XRAY_UUID` | random on first run | VLESS client UUID |
| `FAKE_SNI` | `api24-normal-alisg.tiktokv.com#Tiktok` | Comma-separated `sni#remark` list |
| `WS_PATH` | `/tiktok4g` | WebSocket path |
| `WS_HOST` | `trycloudflare.com` | Overridden by the detected quick-tunnel hostname unless a named tunnel is configured |
| `TRANSPORT` | `websocket` | Only `websocket` is supported in v1 |
| `WEBHOOK_URL` | `""` | Overwritten automatically when cfdeploy is active |
| `TUNNEL_TOKEN` | `""` | Set for a named (production) tunnel; leave blank for a quick tunnel |
| `CLOUDFLARE_API_TOKEN` | `""` (feature off) | Enables the Worker bridge auto-deploy — see permissions below |
| `CLOUDFLARE_ACCOUNT_ID` | `""` (auto-resolve) | Only needed if the token can see more than one account |
| `DOMAIN` | `""` (feature off) | Zone apex only, e.g. `example.com` — **not** `vless.example.com`; the app prepends the `vless.` prefix itself |
| `WORKER_PASSWORD` | random on first run | Becomes the deployed Worker's `/setapi?password=` value |
| `LOG_PASSWORD` | random on first run | Basic Auth for the dashboard/log viewer (`--log-port`, default 9999). Required whenever the dashboard is enabled — see decision log below |

### `CLOUDFLARE_API_TOKEN` permissions

| Permission | Scope | Level |
|---|---|---|
| Workers Scripts | Account | Edit |
| Workers KV Storage | Account | Edit |
| Workers Routes | Zone | Edit |
| Zone | Zone | Read |

## Decision log — system dashboard (`internal/logserver` /stats)

Full plan: `plans/260829-1618-system-dashboard/` (red-teamed, validated,
implemented 2026-08-29).

- **Traffic source**: xray-core's own `app/stats.Manager`, in-process via
  `core.Instance.GetFeature`. Every VLESS client (every port) shares one
  `email: "default"` — xray-core's dispatcher naturally sums all ports into
  one aggregate counter pair (`Engine.Traffic()`), since this is a
  single-user deployment (no per-inbound/per-client breakdown).
- **Counters registered eagerly** at `Engine.New()` time
  (`GetOrRegisterCounter`), not looked up lazily per poll — `ok` from
  `Traffic()` is stable for the whole process lifetime once stats activate,
  avoiding a false-negative on partial registration.
- **Tunnel status**: `tunnel.Supervisor.OnReady` fires on every `/ready`
  poll outcome *and* on all three `watch()` exit paths (process crash,
  closed poll channel, normal poll) — a crash mid-run must not leave the
  dashboard showing a stale "ready" state through the restart/backoff cycle.
- **Update mechanism**: `setInterval` + `fetch("/stats")` every 2s from the
  browser, matching the existing `/logs` 1s-poll idiom — no WebSocket/SSE,
  no added frontend dependency (plain `<canvas>` sparkline).
- **History**: fixed 150-sample in-memory ring (5 min @ 2s), lost on
  restart — no DB, matches this repo's existing `Ring` (logs) pattern.
- **`LOG_PASSWORD` is now required** whenever `--log-port > 0` (the
  default) — `/stats` exposes live traffic rates and the tunnel hostname,
  materially more sensitive than log text alone. `fromEnv()` falls back to
  a freshly generated random secret (not a fixed empty default) when the
  key is absent, so both a brand-new `.env` and an existing pre-dashboard
  `.env` upgrading to this binary still start — same pattern as
  `WORKER_PASSWORD`/`XRAY_UUID`. `--log-port=0` remains a valid
  no-password way to disable the dashboard entirely.
- **Xray liveness (`xray_up`)**: starts as a boot flag (`SetXrayUp`, reset
  on graceful shutdown), then a 5s ticker in `main.go` upgrades it to a
  real signal via `xraycore.ProbeListener` (raw TCP connect-and-close
  against the first configured VLESS port) + `StatusStore.RecordLiveness`,
  which debounces: 2 consecutive failed probes to flip `false`, 1 success
  to flip back `true` immediately.
- **Known limitation, accepted as-is (code review, 2026-08-29)**: tunnel
  hostname (`OnHostname`, parsed from cloudflared stdout) and tunnel-ready
  status (`OnReady`, polled from `/ready`) update via two independent
  goroutines. On a quick-tunnel crash/reconnect, the dashboard can briefly
  show `tunnel_ready: true` paired with the previous session's hostname
  until the new one is parsed. Self-corrects within one poll/parse cycle;
  not fixed — synchronizing the two would add real complexity for a
  sub-second, self-healing local-dashboard display glitch.

## Known limitations

- **android/arm64**: `GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build` fails
  at link time (`link: github.com/wlynxg/anet: invalid reference to
  net.zoneCache`) — a transitive dependency (`xray-core → infra/conf →
  transport/internet/finalmask/realm → github.com/pion/stun/v3 →
  github.com/pion/transport/v4/stdnet → github.com/wlynxg/anet`) pulled in
  unconditionally because `infra/conf` compiles xray-core's entire config
  schema, not just the VLESS-inbound/freedom-outbound subset this project
  uses. Upstream compatibility gap, not fixable in this repo. Not a
  regression for Termux either — `cloudflared` itself doesn't run directly
  under Termux (users are pointed at `pkg install cloudflared` instead).
- KV namespace listing (`cfdeploy/kv.go`) has no pagination — fine at
  personal-account scale, not revisited.
