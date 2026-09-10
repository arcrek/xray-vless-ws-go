---
title: "Rename vless_info and Public URL Subscription QR"
description: "Rename frp_info to vless_info and serve subscription via public URL instead of raw base64 QR"
status: completed
priority: P1
effort: 4h
branch: main
tags: [proxy, vless, subscription, qr, logserver, cfdeploy, web]
blockedBy: []
blocks: []
created: 2026-09-10
---

# Rename vless_info and Public URL Subscription QR

## Overview

This plan addresses two architectural improvements in the `xray-vless-ws-go` proxy system:
1. **Config Renaming**: Rename `frp_info.config` and `frp_info.json` to `vless_info.config` and `vless_info.json`. The legacy `frp_` naming was inherited from older Termux reverse proxy setups; in an Xray VLESS system, "frp" causes confusion (especially on Android where FRP stands for Factory Reset Protection).
2. **Public Subscription URL & Compact QR Code**: Transition the subscription mechanism from a raw 2KB Base64 data dump embedded in the QR canvas to a public, auto-updating Subscription URL (`https://vless.<DOMAIN>/sub` via Cloudflare Worker KV or `/sub` on the embedded logserver). The QR code will encode the Subscription URL instead of giant Base64 strings, producing a clean, low-density QR code that standard V2Ray/Sing-box clients can scan and auto-sync.

## Architecture Diagram

```
+-------------------------------------------------------------------------+
|                           xray-vless-ws-go                              |
|                                                                         |
|  +------------------+         Export         +-----------------------+  |
|  | internal/linkgen | ---------------------> | vless_info.config     |  |
|  |                  |                        | vless_info.json       |  |
|  +------------------+                        +-----------------------+  |
|           |                                              |              |
|        Webhook                                        Read              |
|           v                                              v              |
|  +------------------+                        +-----------------------+  |
|  | internal/cfdeploy|                        | internal/logserver    |  |
|  | (Cloudflare edge)|                        |                       |  |
|  +------------------+                        +-----------------------+  |
+-----------|----------------------------------------------|--------------+
            |                                              |
      Deploy & Sync                                    Serve /sub
            v                                              v
+-----------------------+                      +-----------------------+
|   Cloudflare Worker   |                      |  Embedded Logserver   |
|   vless.<DOMAIN>/sub  |                      |      :9999/sub        |
|  (Public Subscription)|                      |  (Local Subscription) |
+-----------------------+                      +-----------------------+
            \                                              /
             \------- Subscription URL (Compact QR) ------/
                                   |
                                   v
             +-------------------------------------------+
             |            Web Dashboard & QR             |
             |  - Scannable compact Subscription URL QR  |
             |  - 1-click Copy Subscription URL button   |
             |  - Fallback raw Base64 copy button        |
             +-------------------------------------------+
                                   |
                                   v
             +-------------------------------------------+
             |      V2Ray / Sing-box Mobile Clients      |
             | (v2rayNG, Shadowrocket, Sing-box, V2RayN) |
             |   Auto-syncs nodes via Subscription URL   |
             +-------------------------------------------+
```

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Rename frp_info to vless_info](./phase-01-rename-config-files.md) | Completed |
| 2 | [Public Subscription Endpoint Architecture](./phase-02-public-sub-endpoint.md) | Completed |
| 3 | [Dashboard UI Overhaul for Subscription URL & QR](./phase-03-dashboard-qr-and-sub-url.md) | Completed |
| 4 | [Verification, Testing & Documentation](./phase-04-verification-and-docs.md) | Completed |

## Success Criteria

- [x] All occurrences of `frp_info.config` and `frp_info.json` are migrated to `vless_info.config` and `vless_info.json` across code, tests, and web UI, with backward compatibility fallback.
- [x] Public `/sub` subscription endpoint is available both via Cloudflare Worker (`https://vless.<DOMAIN>/sub`) and embedded logserver (`/sub`).
- [x] Dashboard `/api/vless-info` provides `sub_path` and `worker_sub_url` (forming `sub_url`).
- [x] Dashboard Subscription card encodes the Subscription URL into the QR code canvas (clean, low density, instantly scannable) and provides a 1-click "Sao chép URL Subscription" button.
- [x] The raw Base64 string copy button is preserved as a secondary option for offline imports.
- [x] All Go tests (`go test ./...`) pass and documentation (`README.md`, `architecture.md`) reflects the changes.
