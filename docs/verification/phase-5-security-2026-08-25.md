# Phase 5 collaboration security verification

Date: 2026-08-25

This is focused evidence for Phase 5 Task 4. It does not establish full Phase 5 acceptance; observability, updater hardening, and the complete fault/CI matrix remain.

## Implemented

- Centralized service-owned limits copied at construction for HTTP bytes, WebSocket bytes, operation change count, concurrent connections, durable/presence queue depths, limiter entries, and join/durable/presence rates.
- Retained the protocol's immutable identifier, collection, coordinate, and frame ceilings and the existing bounded I/O timeouts.
- Added a pre-upgrade concurrent WebSocket cap and a bounded per-IP join limiter.
- Added bounded per-actor durable-operation and presence limiters. Fixed-window entries expire and the map refuses new keys at its configured cardinality bound.
- Kept actor identity and role authoritative: the authenticated principal replaces client actor IDs, and viewers cannot mutate documents.
- Added stable close code 4429 for rate violations, 4408 for durable slow consumers, 1008 for malformed/policy traffic, and 1009 for oversized frames.
- Malformed client messages close on the first violation. Close reasons and reported errors contain generic scope plus actor/session identifiers, not tokens, headers, or map content.
- Presence remains lossy/coalesced and isolated from durable acknowledgement. The safe default permits the established 100-update pressure case while explicitly configured limits can be stricter.

## Abuse coverage

- Unauthorized or stale WebSocket credential: rejected before upgrade.
- Unapproved origin and missing protocol: rejected before upgrade.
- Connection and per-IP join floods: HTTP 429.
- Forged operation actor: replaced with the authenticated actor.
- Viewer durable operation: rejected with revision unchanged.
- Configured HTTP, WebSocket, and operation-change limits: rejected before mutation.
- Invalid coordinates: stable policy close.
- Per-actor durable and presence floods: stable rate-limit close with authoritative revision unchanged.
- Closed/overflowed durable subscriber queue: stable slow-consumer close.
- Limiter cardinality and idle-key expiry: deterministic unit coverage.

## Verification

- Each new enforcement slice was introduced with a focused failing test and rerun to green.
- `go test ./internal/aphelion/collab/protocol ./internal/aphelion/collab/server -count=1`: pass.
- `go test -race ./internal/aphelion/collab/protocol ./internal/aphelion/collab/server -count=1`: pass.
- Client-envelope fuzz campaign: 429,253 executions in approximately six seconds; pass with no crash.
- Server-envelope fuzz campaign: 433,376 executions in approximately six seconds; pass with no crash.
- `task verify` with Go 1.25.13 and Rust 1.82 GNU: pass, including the static Windows desktop build.
- `golangci-lint run ./...`: `0 issues.`
- Full server race run: pass, including all legacy and Task 4 tests.
- `git diff --check`: pass; Git reports only the repository's existing LF-to-CRLF conversion warnings.

The inherited ImGui/GCC `memset` warning remains unchanged.
