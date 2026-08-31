# Multiplayer implementation readiness

Status date: 2026-08-31

## Decision

The remediation scope in `2026-08-30-admm-audit-remediation.md` is implemented and passes fresh local repository, race, lint, vulnerability, Rust, contract, build, doctor, smoke, workflow-policy, and container gates. The working tree is suitable for human desktop acceptance and further integration testing.

This is not production or Meridian integration acceptance. The exact tested source base was `c3469c60d5cd39c21268e14ee38643910601b2e9` plus the uncommitted remediation diff.

## Implemented and locally verified

- Snapshot dimensions and cell counts are bounded before allocation at model, adapter, protocol, and hosted HTTP boundaries.
- Ambiguous durable appends reconcile against the store before acknowledgement; duplicate IDs with different operations are rejected.
- Export checkpoints are owner-authorized, revision/hash-preconditioned, idempotent, persisted by memory/SQLite/PostgreSQL stores, and have a terminal completion contract. SQLite restart/completion passed; live PostgreSQL did not run.
- `apheliondmm-meridian-verify` composes manifest loading, immutable staging, MCP inspection, and fixed PowerShell acceptance. Fake-process command tests and race tests passed. No installed `meridian-mcp.exe` was available for real cross-stack acceptance.
- Windows file-backed hosted secrets reject broad, null, unreadable, or unsupported DACLs and accept owner/LocalSystem/Administrators-only read access.
- UUIDv7 model IDs use only the standard library and remain unique and lexically nondecreasing under the race detector.
- Inherited implementation changes have `APHELION EDIT` ownership markers; fresh StrongDMM upstream drift is recorded separately.
- Release publication depends on every current quality/security job and every external action is pinned to a reviewed commit SHA.
- Snapshot persistence no longer hot-retries immediately after a write failure; a repeated server-package run exposed and verified this final repair.

## Fresh automated evidence

| Gate | Result |
| --- | --- |
| `go test ./... -count=1` | Passed; inherited ImGui C++ `memset` warning remained |
| `go test -race ./internal/aphelion/... -count=1` | Passed |
| `golangci-lint run` | `0 issues.` |
| `govulncheck ./...` | Zero called or imported-package vulnerabilities; seven required-module-only findings |
| `task verify` with `RUST_TARGET=1.82.0-x86_64-pc-windows-gnu` | Passed semantic contracts, Go, Rust, rustfmt, Clippy, release parser build, and Windows desktop build |
| `go run ./cmd/apheliondmm-doctor` | Go 1.25.13, Rust 1.82.0, Task 3.53.1, golangci-lint 2.12.2, and GCC 15.2.0 reported `ok` |
| `go run ./cmd/apheliondmm-smoke` | Passed parser save/reparse and authenticated two-client convergence |
| `actionlint` and workflow policy tests | Passed |
| `git diff --check` | Passed; checkout line-ending notices only |
| Docker `TestHostedImageLifecycle` | Passed in 8.23 seconds against `sha256:a4f9e3a0176a8ee412e40c3f86f8d9d6e2dd5c96baf21ee6b1536555f0f4b9ed` |
| Trivy 0.74.0 image scan | Passed with zero HIGH/CRITICAL findings in Debian and both Go binaries |

## Unrun gates

- Live PostgreSQL conformance and logical backup/restore: no `APHELION_POSTGRES_TEST_DSN` or `APHELION_POSTGRES_BIN`.
- Real Meridian-MCP, Meridian-Rift build, and Content Tools coordinator: no installed `meridian-mcp.exe`.
- Reference 25-editor pilot, slow-consumer/presence fault run, and hosted latency percentiles: no pilot endpoint.
- External OTLP collector and external OIDC provider: no configured endpoints or credentials. The Docker lifecycle did exercise local TLS OIDC and OTLP fixtures.
- Hosted GitHub Actions: not triggered from the uncommitted working tree.
- All real desktop visual, keyboard, layout, two-window, save/reopen, and unknown-type human acceptance steps.

## Remaining decision gates

Production remains blocked on external OIDC/DNS/tunnel setup, reference-server backup/restore and fault evidence, named-user pilot evidence, updater minisign key ownership/publication, and human desktop acceptance. Multi-replica hosting and public session discovery remain unsupported.
