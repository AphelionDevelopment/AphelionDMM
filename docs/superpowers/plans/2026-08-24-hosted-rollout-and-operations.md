# Hosted Rollout and Operations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Operate the same authoritative collaboration semantics as a secure, recoverable hosted service with PostgreSQL, OIDC, controlled deployment, load/fault evidence, and staged rollout.

**Architecture:** Add PostgreSQL behind the existing store interface, OIDC behind the existing principal/role boundary, TLS at the service edge, and immutable deployment configuration. Preserve protocol semantics across rolling upgrades and retain embedded mode.

**Tech Stack:** Go 1.24, PostgreSQL, `github.com/jackc/pgx/v5`, `github.com/coreos/go-oidc/v3`, OpenTelemetry, OCI containers, HTTPS/WSS.

**Spec:** `docs/superpowers/specs/2026-08-24-multiplayer-design.md`

## Global Constraints

- Read `docs/agent/security-and-networking.md`, `docs/agent/verification.md`, and `docs/agent/generated-and-external-assets.md`.
- Hosted behavior may strengthen identity/durability but may not alter operation semantics.
- Deployment, release, signing, migration, backup, and CI files are protected infrastructure and require exact-file approval before implementation.
- Do not commit, push, publish images, create cloud resources, send invitations, or change external systems without explicit authorization.
- Keep the deployment vendor-neutral; no public session discovery.

---

### Task 1: Implement the PostgreSQL store

**Files:**
- Create: `internal/aphelion/collab/store/postgres/store.go`
- Create: `internal/aphelion/collab/store/postgres/migrations.go`
- Create: `internal/aphelion/collab/store/postgres/schema/001_initial.sql`
- Create: `internal/aphelion/collab/store/postgres/store_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [x] Write the shared store conformance suite against an isolated PostgreSQL test database and confirm failure for the absent implementation.
- [x] Add pinned `github.com/jackc/pgx/v5` modules after license/advisory review.
- [x] Implement transactionally contiguous revisions, unique operation IDs, snapshots, replay, and migration locks.
- [x] Use explicit statement timeouts, bounded pools, context cancellation, and serializable or row-locked document revision assignment.
- [x] Test concurrent submissions from multiple service instances, transaction abort, connection loss, retry classification, and migration contention.
- [x] Compare recovered snapshot/hash results with the SQLite implementation using the same fixtures.
- [ ] If authorized, commit with `feat(store): add PostgreSQL collaboration persistence`.

### Task 2: Add hosted OIDC authentication and role mapping

**Files:**
- Create: `internal/aphelion/collab/auth/oidc.go`
- Create: `internal/aphelion/collab/auth/oidc_test.go`
- Create: `internal/aphelion/collab/auth/session.go`
- Create: `internal/aphelion/collab/auth/session_test.go`
- Modify: `internal/aphelion/collab/server/http.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [x] Write tests with a local signed OIDC issuer for discovery, issuer/audience/signature/expiry/nonce checks, PKCE state binding, key rotation, disabled user, role change, logout, and WebSocket token expiry.
- [x] Run auth tests and confirm failure.
- [x] Add pinned `github.com/coreos/go-oidc/v3` and OAuth2 dependencies.
- [x] Implement Authorization Code with PKCE in the trusted desktop/backend flow; never use an implicit flow or persist refresh tokens in map/project files.
- [x] Map immutable provider subject to internal principal/actor identity. Store display metadata separately.
- [x] Reauthorize session role on join and sensitive owner operations; close connections whose session authorization is revoked.
- [ ] Run focused tests, race tests, and manual login/logout against a non-production test issuer.
- [ ] If authorized, commit with `feat(auth): add hosted OIDC and session roles`.

### Task 3: Define immutable hosted configuration and container runtime

**Files:**
- Create after approval: `deploy/container/Dockerfile`
- Create after approval: `deploy/container/entrypoint.ps1`
- Create after approval: `deploy/config/apheliondmm-collab.example.yaml`
- Create: `internal/aphelion/collab/server/hosted_config.go`
- Create: `internal/aphelion/collab/server/hosted_config_test.go`
- Create after approval: `.dockerignore`

- [x] Write strict config tests for bind address, public origin, proxy trust, database DSN source, OIDC issuer/client ID, limits, telemetry endpoint, and secret-file permissions.
- [x] Run config tests and confirm failure.
- [x] Implement strict YAML decoding with unknown-field rejection and environment indirection only for named secrets.
- [ ] Present exact deployment files/effects and obtain protected-infrastructure approval.
- [ ] Build a non-root, read-only-root-filesystem OCI image with a pinned base digest, health checks, no shell package manager at runtime, and a writable data/temp mount only where required.
- [ ] Terminate TLS in a documented trusted reverse proxy or directly in the service; in either case validate forwarded headers only from configured proxy ranges.
- [ ] Scan the built image, start it locally against ephemeral PostgreSQL/OIDC fixtures, and prove clean graceful shutdown.
- [ ] If authorized, commit with `deploy: add hardened hosted service container`.

### Task 4: Implement backup, restore, and migration runbooks

**Files:**
- Create after approval: `deploy/runbooks/backup.md`
- Create after approval: `deploy/runbooks/restore.md`
- Create after approval: `deploy/runbooks/migrations.md`
- Create: `internal/aphelion/collab/store/postgres/restore_test.go`

- [x] Define recovery point and recovery time targets for the pilot: at most five minutes of unacknowledged exposure and a 60-minute service restoration target. Acknowledged operations remain governed by transaction durability.
- [x] Add an automated restore test that loads a logical backup into an empty database, opens the collaboration store, replays, and confirms document revisions/hashes. Full service startup is covered separately by the container fixture gate.
- [x] Test restoration to a point before and after a known operation and record the expected visible revision.
- [ ] Document pre-migration backup, migration lock, compatibility window, rollback/roll-forward decision, and integrity queries.
- [ ] Ensure backup commands read secrets from the deployment secret mechanism and redact output.
- [x] Run a timed restore rehearsal and record actual duration/artifact hashes.
- [ ] If authorized, commit with `ops: document and test collaboration recovery`.

### Task 5: Add protocol compatibility and rolling-upgrade gates

**Files:**
- Create: `internal/aphelion/collab/compat/matrix.go`
- Create: `internal/aphelion/collab/compat/matrix_test.go`
- Create: `testdata/collaboration/compat/v1/*.json`
- Create: `cmd/apheliondmm-compat/main.go`

- [ ] Write tests for current/current, previous/current, current/previous, unsupported protocol, additive optional field, changed required semantics, and snapshot schema upgrade. Current-version, rolling-pair, rejection, and v1 fixture coverage is present; a real schema-upgrade fixture remains.
- [x] Run compatibility tests and confirm failure.
- [x] Implement an explicit matrix used by `/v1/version`, join negotiation, and the compatibility command.
- [x] Preserve golden accepted/rejected message fixtures and expected map hashes for every supported version.
- [ ] Start old/new service instances against the same PostgreSQL schema only when the matrix permits it; prove operation continuity during a rolling restart.
- [x] Make incompatible versions fail before session join or migration.
- [ ] If authorized, commit with `test(protocol): gate rolling compatibility`.

### Task 6: Build deterministic load and fault tooling

**Files:**
- Create: `internal/aphelion/collab/load/scenario.go`
- Create: `internal/aphelion/collab/load/scenario_test.go`
- Create: `cmd/apheliondmm-loadtest/main.go`
- Create: `testdata/collaboration/load/pilot.json`

- [x] Write tests for deterministic scenario generation from a recorded seed, bounded clients/operations/presence, expected final revision, and expected canonical hash.
- [x] Run load tests and confirm failure.
- [x] Implement a client that uses only public HTTP/WSS contracts and emits machine-readable latency/error/convergence results.
- [x] Define the pilot gate: 25 concurrent editors on one representative map, 10 durable operations per second aggregate, 20 presence updates per second per active editor before coalescing, zero acknowledged-operation loss, zero divergence, and p95 accepted-operation acknowledgement below 250 ms on the reference deployment.
- [ ] Inject service restart, one database connection interruption, slow consumer, and presence flood; verify reconnect/recovery and final hash.
- [x] Keep generated load data neutral and structural; do not introduce authored map content.
- [ ] If authorized, commit with `test(load): add deterministic hosted collaboration scenarios`.

### Task 7: Prepare security review and staged rollout

**Files:**
- Create after approval: `deploy/runbooks/security-review.md`
- Create after approval: `deploy/runbooks/incident-response.md`
- Create after approval: `deploy/runbooks/rollout.md`
- Modify after approval: `.github/workflows/ci.yml`
- Modify after approval: release/signing configuration identified during execution

- [ ] Threat-model authentication, invitation leakage, WebSocket abuse, operation amplification, map payload bombs, SQL abuse, SSRF, path escape, command execution, secret leakage, dependency compromise, updater compromise, backup theft, and denial of service.
- [ ] Verify every mitigation through a test, configuration assertion, or operational control with an owner and evidence reference.
- [ ] Obtain exact-file approval, then add protected CI/release gates for protocol compatibility, PostgreSQL integration, image scan, signed artifacts, and reproducible service build.
- [ ] Run an internal loopback pilot, then a private hosted pilot with named invited users, then a wider opt-in pilot. Keep public discovery disabled.
- [ ] Require successful backup/restore rehearsal, load/fault gate, OIDC revocation test, cross-repository integration gate, and rollback rehearsal before each expansion.
- [ ] Record pilot incidents, p50/p95/p99 acknowledgement latency, reconnect rate, conflicts, store health, and recovery evidence. Stop expansion on data loss, divergence, authorization bypass, or unrecoverable compatibility failure.
- [ ] If authorized, commit with `ops: define secure hosted rollout gates`.

## Phase acceptance

- [ ] PostgreSQL passes the same conformance/hash suite as SQLite.
- [ ] OIDC and roles resist token, issuer, audience, nonce, expiry, and revocation failures.
- [ ] The container runs non-root with immutable configuration and no embedded secrets.
- [ ] Backup restoration meets the pilot recovery targets and exact map hashes.
- [ ] Supported rolling versions maintain contiguous revisions; incompatible versions fail before join.
- [ ] Load/fault scenarios meet the stated pilot gate without acknowledged loss or divergence.
- [ ] Security and rollback rehearsals pass before any deployment expansion.
