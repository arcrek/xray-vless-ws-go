# Phase 2: Public Subscription Endpoint Architecture

## Context Links
- Plan index: [plan.md](./plan.md)
- Embedded HTTP server: `internal/logserver/server.go`
- Cloudflare Worker asset: `internal/cfdeploy/assets/worker.js`
- Main entrypoint & webhook dispatch: `cmd/xrayws/main.go`

## Overview
- **Priority**: P1 (Core Functionality)
- **Status**: Completed
- **Description**: Implement public Subscription endpoints so proxy clients (v2rayNG, Shadowrocket, V2RayN, Sing-box) can fetch and auto-refresh node configurations via standard HTTP/HTTPS GET requests. This removes the dependency on scanning raw Base64 data strings directly. Two complementary endpoints will be established:
  1. **Logserver Endpoint (`/sub`)**: Built into the local Go binary, accessible at `http://<HOST>:<LOG_PORT>/sub`.
  2. **Cloudflare Worker Endpoint (`https://vless.<DOMAIN>/sub`)**: Deployed to Cloudflare's global CDN edge with valid TLS and KV persistence, requiring zero port forwarding on the server.

## Key Insights
1. **Standard V2Ray Subscription Specification**:
   - HTTP GET returns a Base64-encoded string of newline-separated `vless://` URLs.
   - `Content-Type: text/plain; charset=utf-8`.
   - Headers such as `Profile-Update-Interval: 24` indicate that client apps should refresh nodes every 24 hours.
   - `Subscription-Userinfo: upload=0; download=0; total=1073741824000; expire=0` signals node traffic metadata to client apps (e.g. Shadowrocket/v2rayNG).
2. **Existing Webhook Mechanism**:
   - When the tunnel establishes, `cmd/xrayws/main.go` already calls `linkgen.SendWebhook(cfg.WebhookURL, ...)` with a payload containing `"payloads": links`.
   - The Cloudflare Worker's `/setapi` currently ignores `"payloads"`. By storing this in Workers KV (`SUBSCRIPTION`), the Worker can serve `/sub` globally without any extra traffic hitting the origin server!
3. **Public Access vs Protected Dashboard**:
   - The dashboard APIs (`/logs`, `/stats`, `/api/vless-info`) remain protected with session authentication (`withAuth`).
   - The `/sub` endpoint MUST be public (unprotected) so client apps can query it without cookie-based logins.
   - To support flexible client workflows, `/sub?raw=1` or `/sub?raw=true` will return plain-text `vless://` links instead of Base64.

## Architecture & Flow

```
[Proxy Client (v2rayNG / Shadowrocket)]
               |
               | HTTP GET /sub
               +----------------------------------+
               |                                  |
               v                                  v
+------------------------------+   +------------------------------+
| Cloudflare Worker Edge       |   | Embedded Logserver           |
| https://vless.<DOMAIN>/sub   |   | http://<IP>:<LOG_PORT>/sub   |
|                              |   |                              |
| - Reads from KV (cached)     |   | - Reads vless_info.config    |
| - Cloudflare TLS (port 443)  |   | - Fallback to frp_info.config|
| - Fast global CDN edge       |   | - Local / Direct access      |
+------------------------------+   +------------------------------+
               |                                  |
               +----------------+-----------------+
                                |
                                v
               [HTTP 200 text/plain (Base64)]
               Profile-Update-Interval: 24
               vless://uuid@host:443... (b64)
```

## Related Code Files

| File Path | Action | Description |
|---|---|---|
| `internal/logserver/server.go` | Modify | Add `/sub` route, `handleSubscription` handler, and `Domain` field |
| `internal/logserver/server_test.go` | Modify | Add unit tests for `/sub` (base64, raw query, 503 on missing file) |
| `cmd/xrayws/main.go` | Modify | Inject `cfg.Domain` into `logserver.Server` |
| `internal/cfdeploy/assets/worker.js` | Modify | Parse `payloads` in `/setapi`, save to KV, serve `/sub` publicly |
| `internal/cfdeploy/deploy_test.go` | Modify | Ensure worker template tests pass with the new `/sub` handling |

## Implementation Steps

1. **Implement `/sub` Handler in `internal/logserver/server.go`**:
   - Add struct field `Domain string` to `logserver.Server`.
   - Register public routes in `s.Start()`:
     ```go
     mux.HandleFunc("/sub", s.handleSubscription)
     mux.HandleFunc("/subscription", s.handleSubscription)
     ```
   - Implement `handleSubscription(w http.ResponseWriter, r *http.Request)`:
     ```go
     cfgPath := s.ConfigPath
     if cfgPath == "" {
         cfgPath = "vless_info.config"
     }
     content, err := os.ReadFile(cfgPath)
     if err != nil {
         // Graceful fallback to legacy file if present
         if fb, errFb := os.ReadFile("frp_info.config"); errFb == nil {
             content = fb
         } else {
             http.Error(w, "Subscription not ready", http.StatusServiceUnavailable)
             return
         }
     }
     w.Header().Set("Content-Type", "text/plain; charset=utf-8")
     w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
     w.Header().Set("Profile-Update-Interval", "24")
     w.Header().Set("Content-Disposition", `inline; filename="vless_subscription.txt"`)
     w.Header().Set("Subscription-Userinfo", "upload=0; download=0; total=1073741824000; expire=0")

     if r.URL.Query().Get("raw") == "1" || r.URL.Query().Get("raw") == "true" {
         w.Write(content)
         return
     }
     b64 := base64.StdEncoding.EncodeToString(content)
     w.Write([]byte(b64))
     ```
   - Update `handleVlessInfo` to include subscription paths and full URLs in JSON:
     ```go
     resp["sub_path"] = "/sub"
     if s.Domain != "" {
         resp["worker_sub_url"] = fmt.Sprintf("https://vless.%s/sub", s.Domain)
     }
     ```

2. **Pass Domain to Logserver in `cmd/xrayws/main.go`**:
   - In `run()` when instantiating `logSrv`:
     ```go
     logSrv = logserver.New(fmt.Sprintf("0.0.0.0:%d", logPort), cfg.LogPassword, 500)
     logSrv.Domain = cfg.Domain
     ```

3. **Update Cloudflare Worker (`internal/cfdeploy/assets/worker.js`)**:
   - Add global in-memory cache:
     ```javascript
     let GLOBAL_SUB = "";
     ```
   - In `/setapi`:
     ```javascript
     const payloads = jsonBody.payloads;
     if (Array.isArray(payloads) && payloads.length > 0) {
       const subContent = btoa(payloads.join("\n"));
       GLOBAL_SUB = subContent;
       if (env.KV_CONFIG) {
         await env.KV_CONFIG.put("SUBSCRIPTION", subContent);
       }
     }
     ```
   - Add public `/sub` route in Worker `fetch()`:
     ```javascript
     if (url.pathname === "/sub" || url.pathname === "/subscription") {
       let sub = GLOBAL_SUB;
       if (!sub && env.KV_CONFIG) {
         sub = await env.KV_CONFIG.get("SUBSCRIPTION");
         if (sub) GLOBAL_SUB = sub;
       }
       if (!sub) {
         return new Response("Subscription not ready", { status: 503 });
       }
       return new Response(sub, {
         status: 200,
         headers: {
           "Content-Type": "text/plain; charset=utf-8",
           "Cache-Control": "no-cache, no-store, must-revalidate",
           "Access-Control-Allow-Origin": "*",
           "Profile-Update-Interval": "24",
           "Content-Disposition": 'inline; filename="vless_subscription.txt"',
           "Subscription-Userinfo": "upload=0; download=0; total=1073741824000; expire=0"
         }
       });
     }
     ```

4. **Write Unit Tests for `/sub`**:
   - Test default Base64 response.
   - Test `?raw=1` plain-text response.
   - Test 503 Service Unavailable when no config file exists.
   - Test fallback to `frp_info.config` when `vless_info.config` is missing.

## Todo List
- [x] Add `handleSubscription` handler in `internal/logserver/server.go`
- [x] Expose `sub_path` and `worker_sub_url` in `/api/vless-info`
- [x] Wire `cfg.Domain` into `logSrv.Domain` in `cmd/xrayws/main.go`
- [x] Update `internal/cfdeploy/assets/worker.js` with `/setapi` subscription caching and `/sub` endpoint
- [x] Add comprehensive tests for `/sub` in `internal/logserver/server_test.go`
- [x] Run `go test ./...` and verify test suite passes

## Success Criteria
- [x] `GET http://localhost:9999/sub` returns the Base64-encoded subscription with appropriate HTTP headers without authentication.
- [x] `GET http://localhost:9999/sub?raw=1` returns the raw `vless://` links.
- [x] `worker.js` serves `https://vless.<DOMAIN>/sub` using cached subscription data.
- [x] `/api/vless-info` provides `worker_sub_url` (when domain configured) and `sub_path`.

## Risk Assessment
- **Risk**: Unauthenticated `/sub` endpoint exposes node configurations.
- **Mitigation**: This is the intended and standard behavior for V2Ray subscription endpoints. VLESS links already contain the secret UUID. The dashboard's traffic stats, logs, and sensitive management APIs remain strictly protected behind `withAuth`.

## Security Considerations
- Ensure `/sub` only returns the contents of `ConfigPath` (whitelisted file path) and prevents arbitrary file reads.
- Ensure CORS headers (`Access-Control-Allow-Origin: *`) allow web-based subscription managers to fetch nodes without browser cross-origin blocks.
