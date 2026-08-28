# xray-vless-ws-go

Production-ready Go rebuild of [xray_vless_ws_server](https://github.com/vincentng295/xray_vless_ws_server)
(Python PoC for VLESS-WS over Cloudflare Tunnel, DPI-bypass via Anycast/SNI decoupling).

**Status:** scaffold only — implementation not started yet. See the plan:
`/home/arcrek/workspace/4g/plans/260828-1928-xray-go-rebuild/plan.md`

## Key differences from the Python PoC
- `xray-core` embedded as a Go library (in-process, no binary download/subprocess)
- `cloudflared` kept as a managed subprocess (binary downloaded at runtime)
- WARP outbound dropped from v1 (adds latency, against the speed-optimization goal)
- `.env` config format kept for backward compatibility

## Build
Not yet implemented. Run `/ck-skills:cook` against the plan above to implement.
