package xraycore

// Blank-import block: xray-core registers protocol/app handlers via init()
// side effects, so anything used at runtime must be imported here even
// though nothing from these packages is referenced directly.
//
// This list is a minimal subset of xray-core's own
// main/distro/all/all.go (verified against the pinned module source at
// implementation time, not guessed from documentation) — only what VLESS+WS
// inbound with a freedom outbound actually needs:
//   - app/dispatcher, app/proxyman/inbound, app/proxyman/outbound: mandatory,
//     xray-core does not start without them.
//   - app/log: routes xray-core's own log lines through common/log.Handler,
//     which Engine wires to Phase 5's log sink.
//   - proxy/vless/inbound: the only inbound protocol this project uses.
//   - proxy/freedom: the only outbound protocol this project uses (WARP's
//     wireguard outbound is dropped per plan decision log #5).
//   - transport/internet/websocket: the only transport this project uses
//     (TRANSPORT is force-downgraded to "websocket" in internal/config).
//   - main/json: registers the "JSON" config format core.StartInstance("JSON", ...) needs.
//
// app/policy and app/stats are already initialized transitively (via
// main/json -> infra/conf's own non-blank imports of both) — the two blank
// imports below change nothing functionally. Kept only so a reader scanning
// this file's import list can see at a glance that policy/stats support is
// intentionally relied upon, not accidental. (Verified against pinned
// source during planning — see plan phase-01's Key Insights.)
import (
	_ "github.com/xtls/xray-core/app/dispatcher"
	_ "github.com/xtls/xray-core/app/log"
	_ "github.com/xtls/xray-core/app/policy"
	_ "github.com/xtls/xray-core/app/proxyman/inbound"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"
	_ "github.com/xtls/xray-core/app/stats"
	_ "github.com/xtls/xray-core/main/json"
	_ "github.com/xtls/xray-core/proxy/freedom"
	_ "github.com/xtls/xray-core/proxy/vless/inbound"
	_ "github.com/xtls/xray-core/transport/internet/websocket"
)
