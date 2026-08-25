# Phase 5 durable-store verification

Date: 2026-08-25

This is focused evidence for Phase 5 Tasks 1 and 2 plus the completed recovery/snapshot portions and process entry-point persistence of Task 3. It is not full Phase 5 acceptance: the remaining corruption matrix, security limits, telemetry, updater hardening, and fault CI remain.

## Implemented

- Extracted the collaboration store contract into `internal/aphelion/collab/store` while retaining the existing server compatibility API.
- Added reusable conformance coverage for create, duplicate create, contiguous append, idempotent lookup, copy isolation, deterministic replay, compaction, document isolation, cancellation, missing documents, and close.
- Moved the reference memory implementation behind the contract and made store close explicit and idempotent.
- Added `modernc.org/sqlite` v1.57.0 with embedded versioned schemas for documents, snapshots, operations, and retained revision hashes.
- Added transactional create, verified append, duplicate lookup, snapshot compaction, contiguous replay, revision hashes, migration, and close.
- Configured and verifies foreign keys, WAL, 5-second busy timeout, full synchronous mode, and a single bounded connection.
- Refuses SQLite older than 3.51.3. The selected driver returns SQLite 3.53.3.
- Added operation-count and elapsed-time snapshot scheduling off the acknowledgement path.
- Retained snapshots may complete after newer appends; both stores preserve the later contiguous replay instead of rejecting the snapshot or discarding operations.
- Added baseline-authenticated recovery through contiguous replay and canonical operation verification.
- Added durable-store and snapshot configuration injection to the service and embedded APIs. Recovery creates a new authenticated session and does not persist bearer/resumption credentials.
- Added optional trusted-local `-database`, `-snapshot-operation-threshold`, and `-snapshot-interval` command flags. Omitting `-database` preserves the memory-only behavior.
- The command opens and migrates SQLite before binding its listener, then delegates store ownership to the embedded service for orderly shutdown.
- Schema v2 retains an append-only revision-hash ledger so snapshot compaction cannot erase the trusted baseline proof needed for restart. Schema-v1 databases migrate by backfilling hashes from stored snapshots and operations.
- Split liveness from readiness. Unrecoverable documents produce readiness `503` while liveness and unrelated document ownership remain available; successful recovery clears the document-specific failure.

## Verification

- Red gate: SQLite conformance initially failed to compile because `Open` was absent.
- Memory and SQLite conformance: pass.
- Windows on-disk restart/reopen: pass.
- Concurrent duplicate append: pass with exactly one replay entry.
- Locked database: bounded failure with no partial document, followed by successful create after lock release.
- Injected operation insert failure: transaction rolls back; the subsequent append succeeds.
- Future schema version: rejected.
- Schema-v1 revision hashes: migrated and backfilled; initial and accepted revision hashes remain available.
- Protected command entry point: a real WebSocket operation is accepted and acknowledged, the service restarts against the same SQLite file, and revision/hash recovery succeeds. The threshold-configured path also proves the snapshot advances before graceful shutdown.
- Forced process termination: the child command process is killed after acknowledgement and before graceful shutdown; the replacement process recovers the operation exactly once from the same database.
- Credential persistence: the closed SQLite file contains neither the single-use launch token nor the owner bearer token.
- `go test -race ./internal/aphelion/collab/store/... ./internal/aphelion/collab/server -count=1`: pass.
- `go test -race ./internal/aphelion/collab/store/... ./internal/aphelion/collab/server ./cmd/apheliondmm-collab -count=1`: pass after protected entry-point integration.
- golangci-lint 2.12.2: `0 issues.`
- `govulncheck ./...`: zero reachable vulnerabilities and zero vulnerabilities in imported packages.
- `task verify` with Go 1.25.13 and Rust 1.82 GNU: pass, including the static Windows desktop build.
- Recovery, snapshot scheduling, configured-store HTTP recovery, focused lint, focused race, and the complete Go suite pass after Task 3 integration.
- The recovery matrix now covers uncompacted replay, compacted snapshot plus later replay, discontinuity/missing revision, corrupt stored snapshots, mismatched canonical baseline hashes, interrupted snapshot writes, and acknowledged-operation survival.

The inherited ImGui/GCC `memset` warning remains unchanged.
