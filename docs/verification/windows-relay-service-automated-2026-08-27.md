# Native Windows relay service automated verification

**Date:** 2026-08-27

**Revision embedded in package:** `6f2d4fc7`

**Result:** Native service code, operator package, documentation, and protected CI entry points pass all locally available automated gates. The current shell was not elevated, so a real local Service Control Manager install/start/update/uninstall was not executed. That exact lifecycle is encoded in the `relay-windows-service-gates` Windows CI job and remains a required pre-deployment gate.

## Delivered boundary

- `AphelionDMMRelay` runs the relay in-process and supervises one dedicated `cloudflared.exe` child.
- The relay, readiness endpoint, and metrics bind to loopback. The installer creates no inbound firewall rule.
- The service runs as `NT AUTHORITY\LocalService`, uses delayed automatic start and SCM recovery, and writes to the `AphelionDMMRelay` Event Log source.
- The operator entry point supports Setup, Validate, Package, Install, Start, Status, Logs, Update, Stop, and Uninstall.
- Updates retain rollback binaries; connector-token rotation uses a temporary ACL-restricted rollback copy that is removed after success or recovery.
- Conservative uninstall verifies the service executable before removal and preserves data unless `-PurgeData` is explicit.
- Protocol-v2 Dockerfile, Compose overlays, old operator script, environment template, and associated container tests were removed. Legacy protocol-v1 deployment history remains unchanged.

## Toolchain

- Go: `go1.25.13 windows/amd64`
- Rust: `rustc 1.82.0 (f6e511eec 2024-10-15)`
- Task: `3.53.1`
- PowerShell: `7.6.4`
- Cloudflare connector: `2026.7.3`, valid Cloudflare Authenticode signature

## Passed gates

- `go test ./... -count=1`
- `go test -race ./internal/aphelion/... -count=1`
- `task test-rust`
- `task build`
- `task verify-relay`
- `go test ./cmd/apheliondmm-relay ./internal/aphelion/collab/windowsservice ./internal/aphelion/collab/servicehost -count=1 -timeout=5m`
- `actionlint .github/workflows/ci.yml`
- `govulncheck ./...`: no called vulnerabilities
- `trivy fs --scanners secret --severity HIGH,CRITICAL --exit-code 1 --quiet .`: no secret findings
- `git diff --check`
- tracked personal-name scan: zero occurrences
- runtime token/config status scan: zero tracked or untracked secret files

The Go and race builds emitted only the inherited ImGui/GCC `memset` warning. No new warning or failure was observed.

## Package evidence

| Artifact | SHA-256 |
| --- | --- |
| `AphelionDMM-Relay-Windows-x64.zip` | `A34D33B9B0E962BADAE0DB7FA7E6F1EA988083E3540F623B0AFC5EAC5042731A` |
| packaged `apheliondmm-relay.exe` | `2AFD6435741C7BD6CD74C25EB87D341FAE25D805878124335B75802096E5E937` |
| packaged `apheliondmm-healthcheck.exe` | `6260BDDEE17025CE23272E751D857E98A87A7B247765355ACB8C7A4916E97157` |
| pinned `cloudflared-windows-amd64.exe` | `8635DA433B6DF8194746E88ED9D2589566C20E38BFC2A80E431A348B7C765841` |
| `StrongDMM.exe` | `066854A9A93E44FE9204364337CCDBD9C2500EB6C42F597A470B1B793CE47361` |

Package validation executed the packaged relay against the generated YAML, verified both package-binary hashes, verified the immutable connector pin, and accepted the downloaded connector only after SHA-256 and Authenticode checks.

## Remaining gate

The local process reported `IS_ADMIN=False`; `AphelionDMMRelay` was absent and a separate existing `Cloudflared` service was present. No service was created, stopped, or modified. Before public deployment, the Windows CI job or an elevated disposable host must pass:

1. package install with `-SkipCloudflare`;
2. loopback readiness and version;
3. stop/start and readiness;
4. relay update and readiness;
5. conservative uninstall with explicit data purge;
6. confirmation that the pre-existing `Cloudflared` service remains unchanged.

After that automated administrator gate, the remaining work is dedicated-tunnel installation, public `mapping.a13.info` health/WebSocket certification, and the documented two-network human pilot.
