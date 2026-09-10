# Phase 4: Verification, Testing & Documentation

## Context Links
- Plan index: [plan.md](./plan.md)
- All phase files: [Phase 1](./phase-01-rename-config-files.md), [Phase 2](./phase-02-public-sub-endpoint.md), [Phase 3](./phase-03-dashboard-qr-and-sub-url.md)
- Documentation: `README.md`, `docs/architecture.md`

## Overview
- **Priority**: P1 (Verification & Safety)
- **Status**: Completed
- **Description**: Validate the end-to-end functionality across all modified components, ensuring regression-free operation. Verify that the test suite passes, `vless_info.config` and `vless_info.json` are properly written and watched, the public `/sub` endpoint serves valid Base64 payloads with standard subscription headers, and the dashboard UI cleanly renders the compact QR code. Update project documentation to reflect the new file names and subscription architecture.

## Key Insights
1. **Verification Surface**:
   - `internal/linkgen`: `TestBuildLinksGoldenFixture` and `TestExport` must pass with updated fixture filenames.
   - `internal/logserver`: New unit tests for `/sub` covering default Base64 output, `?raw=1` query, 503 fallback, and `/api/vless-info` response structure.
   - `internal/ci`: `TestUploadFile` and `gitpush_test.go` must pass with `vless_info.config`.
   - `internal/cfdeploy`: `TestWorkerAssetHasPasswordMarker` and `deploy_test.go` must pass with updated `worker.js`.
2. **Smoke Testing**:
   - A throwaway verification test will simulate starting `logserver`, generating `vless_info.config`, querying `GET /sub` and `GET /sub?raw=1`, and validating response status, content type, and base64 decode integrity.
3. **Documentation Consistency**:
   - Both `README.md` and `docs/architecture.md` contain tables and narrative references to `frp_info`. These must be consistently updated to `vless_info` to prevent any confusion for future maintainers and users.

## Related Code Files

| File Path | Action | Description |
|---|---|---|
| `internal/logserver/server_test.go` | Modify | Add comprehensive tests for `/sub` endpoint |
| `internal/linkgen/links_test.go` | Modify | Ensure golden fixture test points to `golden_vless_info.config` |
| `internal/linkgen/export_test.go` | Modify | Ensure export tests check `vless_info.config` and `vless_info.json` |
| `internal/ci/gitpush_test.go` | Modify | Ensure CI tests use `vless_info.config` |
| `README.md` | Modify | Document `vless_info.*` and subscription URL feature |
| `docs/architecture.md` | Modify | Document component updates and public `/sub` architecture |

## Implementation Steps

1. **Execute Unit Test Suites**:
   Run unit tests across all packages:
   ```bash
   go test -v ./internal/linkgen/...
   go test -v ./internal/logserver/...
   go test -v ./internal/ci/...
   go test -v ./internal/cfdeploy/...
   go test -v ./...
   ```

2. **Integration Verification of `/sub`**:
   - Create a test verifying that:
     1. When `vless_info.config` contains:
        ```
        vless://uuid@host:443?type=ws&security=tls#TLS
        vless://uuid@host:80?type=ws&security=#NO%20TLS
        ```
     2. `GET /sub` returns HTTP 200 with:
        - `Content-Type: text/plain; charset=utf-8`
        - `Profile-Update-Interval: 24`
        - Base64 payload that decodes back to the exact original lines.
     3. `GET /sub?raw=1` returns the raw lines.
     4. When `vless_info.config` is missing, but `frp_info.config` exists, `/sub` falls back gracefully to `frp_info.config`.
     5. When neither file exists, `/sub` returns HTTP 503 Service Unavailable.

3. **Verify UI Assets & Static Embed**:
   - Ensure `web/logserver/embed.go` embeds all updated HTML/CSS/JS without issues.
   - Validate that the dashboard loads properly and that QR code generation executes without JavaScript errors.

4. **Update Documentation**:
   - In `README.md`:
     - Update references to `frp_info.config` and `frp_info.json` to `vless_info.config` and `vless_info.json`.
     - Add a section explaining the Subscription URL feature and QR code scanning.
   - In `docs/architecture.md`:
     - Update component table for `internal/linkgen`.
     - Add an entry in the decision log detailing why `frp_info` was renamed and why the public Subscription URL was adopted.

## Todo List
- [x] Run full Go test suite: `go test -v ./...`
- [x] Verify unit test coverage for `/sub` (base64, raw, headers, fallback, 503)
- [x] Verify Cloudflare Worker script template validity (`deploy_test.go`)
- [x] Update `README.md` with new file names and subscription instructions
- [x] Update `docs/architecture.md` with architectural decision rationale
- [x] Confirm clean git status (no untracked artifacts or test leftovers)

## Success Criteria
- [x] All package tests pass (`ok`) with zero failures.
- [x] `/sub` endpoint adheres to standard V2Ray subscription specs.
- [x] Documentation is complete, accurate, and completely free of obsolete `frp_info` references (except in historical migration/fallback notes).

## Risk Assessment
- **Risk**: A missing fixture or broken test after renaming files.
- **Mitigation**: Verify tests iteratively in each phase and run the entire suite as the final gate.
