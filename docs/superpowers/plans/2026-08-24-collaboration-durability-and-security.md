# Collaboration Durability and Security Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make acknowledged collaboration state survive restart and add the security, observability, updater, and fault-recovery controls required before LAN or toolset deployment.

**Architecture:** Put transactional storage behind the existing `SessionStore`, use SQLite for embedded/single-host deployments, recover by snapshot plus contiguous replay, and wrap network/process boundaries with bounded policy and telemetry.

**Tech Stack:** Go 1.25.13, SQLite at least 3.51.3 through a pinned Go driver, OpenTelemetry Go, existing HTTP/WebSocket service, structured logs.

**Spec:** `docs/superpowers/specs/2026-08-24-multiplayer-design.md`

> **Status reconciliation (2026-08-25):** Automated local durability, recovery, security, resilience, telemetry, and updater fail-closed work is complete. Remaining work is human desktop interruption testing and production signing ownership. See `2026-08-25-multiplayer-human-test-readiness.md`. Changes remain uncommitted by repository policy.

**Dependency evidence:** `docs/verification/phase-5-dependency-evaluation-2026-08-25.md`

**Protected toolchain evidence:** `docs/verification/toolchain-security-audit-2026-08-25.md`

**Durable-store evidence:** `docs/verification/phase-5-store-2026-08-25.md`

## Global Constraints

- Read `docs/agent/security-and-networking.md` and `docs/agent/verification.md`.
- Acknowledgement follows the configured durable append; tests must kill/restart processes at boundary points.
- Do not expose filesystem/process configuration to clients.
- Updater, Taskfile, CI, release, and signing changes require exact-file user approval.
- Do not commit without explicit authorization.

---

### Task 1: Formalize the durable store contract

**Files:**
- Create: `internal/aphelion/collab/store/store.go`
- Create: `internal/aphelion/collab/store/conformance.go`
- Create: `internal/aphelion/collab/store/memory.go`
- Create: `internal/aphelion/collab/store/memory_test.go`
- Modify: `internal/aphelion/collab/server/document.go`

- [x] Move the phase-3 store interface into `store` and write reusable conformance tests for create, append, duplicate lookup, snapshot, load, continuity, and isolation between documents.

```go
type SessionStore interface {
	Create(ctx context.Context, snapshot model.Snapshot) error
	Append(ctx context.Context, accepted model.AcceptedOperation) error
	Load(ctx context.Context, id model.DocumentID) (model.Snapshot, []model.AcceptedOperation, error)
	SaveSnapshot(ctx context.Context, snapshot model.Snapshot) error
	RevisionHash(ctx context.Context, id model.DocumentID, revision model.Revision) (string, bool, error)
	LookupOperation(ctx context.Context, id model.DocumentID, operationID model.OperationID) (model.AcceptedOperation, bool, error)
	Close() error
}
```

- [x] Run `go test ./internal/aphelion/collab/store -count=1` and confirm failure.
- [x] Implement a reference memory store with copy-on-read/write and deterministic ordering.
- [x] Run the conformance suite against the memory store and rerun server tests.
- [ ] If authorized, commit with `refactor(store): formalize collaboration persistence contract`.

### Task 2: Add transactional SQLite persistence

**Files:**
- Create: `internal/aphelion/collab/store/sqlite/store.go`
- Create: `internal/aphelion/collab/store/sqlite/migrations.go`
- Create: `internal/aphelion/collab/store/sqlite/schema/001_initial.sql`
- Create: `internal/aphelion/collab/store/sqlite/store_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [x] Before adding a driver, record its exact module version, bundled SQLite version, license, supported platforms, and confirmation that `SELECT sqlite_version()` is at least 3.51.3. Reject the dependency if the runtime query is lower.
- [x] Run the store conformance suite against an absent SQLite implementation and confirm failure.
- [x] Add tables for documents, snapshots, and operations with unique `(document_id, operation_id)` and `(document_id, revision)` constraints.
- [x] Implement transactional create, idempotent append/lookup, snapshot save, and contiguous load.
- [x] Configure foreign keys, WAL, busy timeout, bounded connections, and explicit synchronous mode. Record the SQLite version and refuse an unsafe version.
- [x] Test concurrent duplicate append, bounded lock failure, migration idempotency/future-version rejection, rollback on an injected write failure, and database close. Model serialization has no fallible field type, so the rollback gate injects the database write failure after successful serialization.
- [x] Run conformance tests, race tests, and a Windows on-disk test.
- [ ] If authorized, commit with `feat(store): persist collaboration state in SQLite`.

### Task 3: Implement snapshots and restart recovery

**Files:**
- Create: `internal/aphelion/collab/server/snapshotter.go`
- Create: `internal/aphelion/collab/server/recovery.go`
- Create: `internal/aphelion/collab/server/recovery_test.go`
- Modify: `internal/aphelion/collab/server/config.go`

- [x] Write tests for snapshot thresholds, restart from snapshot plus replay, no-snapshot replay, missing revision, corrupt snapshot, wrong hash, interrupted snapshot write, and last acknowledged revision survival.
- [x] Run recovery tests and confirm failure.
- [x] Snapshot on configurable accepted-operation count and elapsed time without blocking the document owner on filesystem work; serialize an immutable revision view.
- [x] Load the latest valid snapshot, replay a contiguous log, and compare canonical hash before opening a document.
- [x] Fail readiness for corrupt/discontinuous documents while keeping unrelated documents available.
- [x] Add a process-level test that acknowledges an operation, terminates the service without a graceful snapshot, restarts, and confirms the operation is present.
- [ ] If authorized, commit with `feat(store): recover documents from snapshot and replay`.

### Task 4: Enforce network and authorization policy

**Files:**
- Create: `internal/aphelion/collab/server/security.go`
- Create: `internal/aphelion/collab/server/limits.go`
- Create: `internal/aphelion/collab/server/security_test.go`
- Modify: `internal/aphelion/collab/server/http.go`
- Modify: `internal/aphelion/collab/server/websocket.go`

- [x] Write abuse tests for unauthorized upgrade, unapproved origin, forged actor ID, role violation, oversized HTTP body, oversized WebSocket frame, excessive changes, invalid coordinates, join floods, operation floods, slow reader, and repeated malformed messages. Malformed frames close on the first violation rather than retaining a connection for repeated violations.
- [x] Run security tests and confirm failure.
- [x] Implement immutable limits for bytes, identifiers, operations, changes, joins, connection count, queue depth, and timeouts.
- [x] Add per-IP join and per-actor durable/presence rate limiters with bounded memory and expiry.
- [x] Derive actor ID and role from the authenticated server-side principal, overwriting any client field before domain validation.
- [x] Close repeated violators with stable close codes and secret-free log fields.
- [x] Run security tests under race detection and with fuzzed envelope decoding.
- [ ] If authorized, commit with `security: enforce collaboration transport and role limits`.

### Task 5: Add OpenTelemetry observability and readiness

**Files:**
- Create: `internal/aphelion/collab/telemetry/telemetry.go`
- Create: `internal/aphelion/collab/telemetry/metrics.go`
- Create: `internal/aphelion/collab/telemetry/telemetry_test.go`
- Modify: `internal/aphelion/collab/server/http.go`
- Modify: `internal/aphelion/collab/server/document.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [x] Write tests using OpenTelemetry in-memory exporters to assert operation, store, replay, presence-drop, and connection spans/metrics without raw map data or token attributes.
- [x] Run telemetry tests and confirm failure.
- [x] Add pinned OpenTelemetry API/SDK modules and initialize them through dependency injection.
- [x] Instrument validation, append, apply, broadcast, reconnect replay, snapshot, and recovery.
- [x] Make `/v1/health/live` process-only and `/v1/health/ready` reflect store migrations and recoverable document ownership.
- [x] Verify disabled telemetry adds minimal allocations and no network dependency.
- [ ] If authorized, commit with `feat(observability): instrument collaboration service`.

### Task 6: Harden updater and outbound HTTP behavior

**Files:**
- Modify after approval: `internal/req/req.go`
- Modify after approval: `internal/app/selfupdate/selfupdate.go`
- Modify after approval: `internal/app/selfupdate/manifest.go`
- Create: `internal/req/req_test.go`
- Create: `internal/app/selfupdate/selfupdate_test.go`

- [x] Write tests for connect/TLS/header/body timeouts, response-size limit, non-success status, invalid content type, bad signature, truncated artifact, and preservation of the installed executable.
- [x] Run focused tests and confirm existing behavior fails the required cases.
- [x] Present the exact updater/request file effects and obtain protected-infrastructure approval.
- [x] Replace default clients with an injected bounded `http.Client`; apply `io.LimitReader` before buffering.
- [x] Verify signed metadata and artifact hash/signature before handing data to the existing replacement mechanism.
- [x] Preserve the old executable and emit a recoverable error on every failed verification/replacement.
- [x] Run updater tests without public network access using `httptest.Server`.
- [ ] If authorized, commit with `security(updater): bound and verify update downloads`.

### Task 7: Add crash, corruption, and saturation gates

**Files:**
- Create: `internal/aphelion/collab/server/fault_test.go`
- Create: `internal/aphelion/collab/store/sqlite/fault_test.go`
- Create: `cmd/apheliondmm-collab/fault_test.go`
- Modify after approval: `.github/workflows/ci.yml`

- [x] Add deterministic fault injection at store append, snapshot write, broadcast, and shutdown boundaries.
- [x] Prove unacknowledged operations may be absent after crash but acknowledged operations are recovered exactly once.
- [x] Corrupt a copied test database/snapshot and prove readiness fails without altering the source fixture.
- [x] Saturate presence and operation queues and prove bounded memory/latency behavior.
- [x] Obtain CI-file approval, then add race, SQLite restart, fuzz smoke, and fault-test jobs using pinned toolchains.
- [x] Run the complete local verification set and the real service/desktop entry points.
- [ ] If authorized, commit with `test(collab): gate durability and failure recovery`.

## Phase acceptance

- [x] SQLite runtime version meets the minimum and all store conformance tests pass.
- [x] Every acknowledged operation survives forced restart exactly once.
- [x] Corrupt or discontinuous state fails closed and does not affect unrelated documents.
- [x] Transport/role abuse is bounded and leaves authoritative state unchanged.
- [x] Telemetry exposes required signals without secrets or map content.
- [x] Updater failures preserve the installed application.
- [x] LAN enablement remains explicit; zero-value configuration is loopback-only.
