# Hosted collaboration security review

## Review rule

No wider pilot begins until every row below has an owner and current evidence. “Planned” is not evidence. Public session discovery remains disabled.

| Threat | Required mitigation | Owner | Evidence |
|---|---|---|---|
| OIDC token forgery or substitution | Signature, issuer, audience, expiry, nonce, state, PKCE, and optional `at_hash` verification; no redirect following | Application | `internal/aphelion/collab/auth/oidc_test.go` |
| Disabled user or role change | Session-scoped authorization on join, each durable message, owner operations, and idle interval; revoke closes socket | Application/identity | `internal/aphelion/collab/auth/session_test.go`, server WebSocket tests |
| Invitation leakage/reuse | Bounded short-lived one-use credentials stored only as hashes, never URLs/logs, explicit editor/viewer role | Application | hosted server and PostgreSQL registry tests |
| WebSocket origin/auth bypass | Exact origin allowlist, bearer auth before upgrade, session join authorization, protocol negotiation | Application/edge | server security/websocket tests |
| Operation amplification | Message/change limits, per-actor rate limits, bounded queues, authoritative actor replacement | Application | limits and security tests |
| Map payload bomb | HTTP/WebSocket byte limits, coordinate/count validation, canonical hash before mutation | Application | protocol/server security tests |
| SQL injection or revision race | Parameterized SQL, validated schema identifier, serializable/row-locked revision assignment | Database/application | PostgreSQL conformance/concurrency tests |
| SSRF | OIDC issuer is immutable HTTPS configuration; no client-controlled server fetch URL | Application | hosted config and OIDC tests |
| Path escape or command execution | Hosted protocol carries document IDs, not paths/commands; fixed executables only in operator workflows | Application/operator | security guide and config tests |
| Secret leakage | File/named-environment indirection, regular-file/permission checks, redacted errors/telemetry, no credentials in argv | Platform/application | hosted config tests and runbooks |
| Dependency compromise | Pinned modules/actions/toolchains, `govulncheck`, advisory review, image scan | Build/release | CI and verification records |
| Updater/release compromise | Checksummed artifacts, commit-pinned Sigstore-backed provenance attestation, least-privilege release token, draft release review, rollback | Release | updater security evidence; updater-compatible minisign publication remains required |
| Backup theft or deletion | Encryption, separate backup identity, immutable retention, restore rehearsal | Database/operator | backup/restore runbooks and restore test |
| Denial of service | Independent join/durable/presence limits, connection caps, timeouts, slow-consumer closure, edge limits | Application/edge | resilience/security tests |

## Known blocking gaps

- Multi-replica accepted-operation fanout is not implemented. The deployment must remain single-replica with stop-then-start replacement.
- A live non-production OIDC login/logout exercise is still required.
- The 25-editor loopback public-contract gate passes, but a hosted reference run with database interruption, process restart, slow consumer, and presence flood is still required.
- The configured telemetry endpoint is validated but no hosted OTLP exporter is wired yet; operators must not assume external metrics are emitted.
- Release binaries and archives receive GitHub/Sigstore build-provenance attestations. Production self-update remains disabled until a human-controlled minisign key signs the strict manifest envelope and every platform artifact; provenance does not substitute for that updater trust root.
