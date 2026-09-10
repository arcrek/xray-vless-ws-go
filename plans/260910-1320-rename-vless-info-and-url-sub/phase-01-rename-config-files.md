# Phase 1: Rename frp_info to vless_info

## Context Links
- Plan index: [plan.md](./plan.md)
- Export implementation: `internal/linkgen/export.go`
- Main entrypoint & CI watcher: `cmd/xrayws/main.go`, `internal/ci/watcher.go`
- Embedded Log Server: `internal/logserver/server.go`

## Overview
- **Priority**: P1 (Core Foundation)
- **Status**: Completed
- **Description**: Rename all occurrences of `frp_info.config` and `frp_info.json` to `vless_info.config` and `vless_info.json`. The term "FRP" is widely associated on Android devices with "Factory Reset Protection" (FRP bypass/lock), creating confusion for mobile/termux users. In this project, the proxy protocol is VLESS over WebSocket, so `vless_info.*` accurately reflects the file contents and aligns directly with the existing `/api/vless-info` endpoint.

## Key Insights
1. **Source of Truth**: `internal/linkgen/export.go` writes the config and json files upon tunnel connection or IP change.
2. **Backward Compatibility**: `internal/logserver/server.go` should check `vless_info.*` first, but fall back to `frp_info.*` if present, ensuring uninterrupted operation for running nodes or legacy configurations.
3. **CI Bridge**: In `cmd/xrayws/main.go`, `ci.WatchAndUpload` monitors file modifications to push to the GitHub `config` branch. Updating this list ensures the CI branch receives `vless_info.config` and `vless_info.json`.
4. **Golden Test Fixture**: `internal/linkgen/testdata/golden_frp_info.config` should be renamed to `golden_vless_info.config` and updated in `links_test.go`.

## Requirements
- **Functional**:
  - `Export()` writes `vless_info.config` and `vless_info.json` by default.
  - `logserver` reads `vless_info.config` and `vless_info.json`, with fallback to `frp_info.*`.
  - `.gitignore` ignores `vless_info.config` and `vless_info.json` (and preserves ignoring `frp_info.*`).
  - Web UI download and copy actions reference `vless_info.config`.
- **Non-Functional**:
  - No breaking changes for callers providing custom file paths.
  - Test suites (`go test ./...`) pass cleanly.

## Related Code Files

| File Path | Action | Description |
|---|---|---|
| `.gitignore` | Modify | Add `vless_info.config` and `vless_info.json` |
| `cmd/xrayws/main.go` | Modify | Update default export paths, CI watch list, and flag help strings |
| `internal/linkgen/export.go` | Modify | Rename internal struct `frpInfo` -> `vlessInfo` and update documentation |
| `internal/linkgen/export_test.go` | Modify | Update test file paths to `vless_info.*` |
| `internal/linkgen/testdata/golden_frp_info.config` | Move | Rename to `internal/linkgen/testdata/golden_vless_info.config` |
| `internal/linkgen/links_test.go` | Modify | Update fixture path reference |
| `internal/logserver/server.go` | Modify | Update default `ConfigPath`/`JSONPath` with fallback logic |
| `internal/logserver/server_test.go` | Modify | Update mock config paths to `vless_info.*` |
| `internal/ci/gitpush_test.go` | Modify | Update test file paths to `vless_info.config` |
| `web/logserver/static/app.js` | Modify | Update download filename and copy toast notifications |
| `web/logserver/static/index.html` | Modify | Update tooltip and button labels |
| `docs/architecture.md` | Modify | Update component table and descriptions |
| `README.md` | Modify | Update CI description and configuration documentation |

## Implementation Steps

1. **Update .gitignore**:
   Add `vless_info.config` and `vless_info.json`.
2. **Update linkgen Package**:
   - In `internal/linkgen/export.go`:
     Rename `type frpInfo struct` to `type vlessInfo struct`.
     Update comments referencing `frp_info.config` and `frp_info.json`.
   - Rename fixture `internal/linkgen/testdata/golden_frp_info.config` -> `golden_vless_info.config`.
   - In `internal/linkgen/links_test.go`:
     Update fixture load path to `testdata/golden_vless_info.config`.
   - In `internal/linkgen/export_test.go`:
     Update test paths from `frp_info.*` to `vless_info.*`.
3. **Update Main Application**:
   - In `cmd/xrayws/main.go`:
     Change `ci.WatchAndUpload` path slice from `[]string{"frp_info.config", "frp_info.json"}` to `[]string{"vless_info.config", "vless_info.json"}`.
     Change `linkgen.Export(links, meta, "frp_info.config", "frp_info.json")` to `"vless_info.config", "vless_info.json"`.
     Update `-ci-mode` flag description.
4. **Update logserver Package**:
   - In `internal/logserver/server.go`:
     Default `ConfigPath` to `"vless_info.config"`, `JSONPath` to `"vless_info.json"`.
     In `handleVlessInfo`: if `vless_info.config` does not exist on disk, check if `frp_info.config` exists as a graceful fallback. Same for `JSONPath`.
   - In `internal/logserver/server_test.go`:
     Update test cases to use `vless_info.config` and `vless_info.json`.
5. **Update Web UI Labels & Download Filename**:
   - In `web/logserver/static/app.js`:
     Change toast `"Đã sao chép nội dung frp_info.config!"` -> `"Đã sao chép nội dung vless_info.config!"`.
     Change download filename `a.download = "vless_info.config"`.
     Change toast `"Đã tải xuống file frp_info.config"` -> `"Đã tải xuống file vless_info.config"`.
   - In `web/logserver/static/index.html`:
     Update button titles for copy and download.
6. **Update Documentation**:
   - Update `README.md` and `docs/architecture.md`.

## Todo List
- [x] Update `.gitignore` with `vless_info.config` and `vless_info.json`
- [x] Rename `frpInfo` to `vlessInfo` in `internal/linkgen/export.go`
- [x] Rename golden fixture and update `internal/linkgen/links_test.go` and `export_test.go`
- [x] Update `cmd/xrayws/main.go` export and CI watch paths
- [x] Update `internal/logserver/server.go` default paths and fallback logic
- [x] Update `web/logserver/static/app.js` and `index.html` text/tooltips/download filename
- [x] Update `README.md` and `docs/architecture.md`
- [x] Run `go test ./...` and verify all tests pass

## Success Criteria
- [x] `vless_info.config` and `vless_info.json` are generated when `xrayws` runs.
- [x] Web UI downloads `vless_info.config`.
- [x] Fallback check in `logserver` reads `frp_info.config` if `vless_info.config` is absent.
- [x] Zero build or test regressions across `go test ./...`.

## Risk Assessment
- **Risk**: External tools or CI workflows expecting `frp_info.config` on the `config` branch.
- **Mitigation**: Implement file fallback in `logserver`. If legacy compatibility in CI branch is needed, `linkgen.Export` can optionally write both or a symlink if configured, though clean cutover is preferred as requested.

## Security Considerations
- Config files contain sensitive UUIDs and tunnel credentials; permissions remain `0o644` (or `0o600`) and gitignored in the working directory.
