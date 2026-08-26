# Collaboration Replay and Identity Repair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repair the observed duplicate-revision terminal failure, add recoverable integrity handling, and let every session participant manage a session-scoped display name.

**Architecture:** Bound replay to an authoritative high-water revision and filter its overlap from the live durable queue. Add client-side exact-duplicate idempotence as defense in depth, route genuine integrity failures into reconnect, and update participant names through authenticated session state rather than trusting presence payloads.

**Tech Stack:** Go 1.24, `nhooyr.io/websocket`, StrongDMM ImGui UI, existing collaboration protocol/server/client packages.

**Spec:** `docs/superpowers/specs/2026-08-26-online-pilot-readiness-design.md`

> **Status reconciliation (2026-08-26):** Tasks 1-5 are implemented, including deterministic replay-overlap coverage, altered-duplicate/hash/gap recovery coverage, hosted name persistence, and deliberate-close normalization. The automated gates and Windows build in Task 6 pass; the remaining work is the two-window human checklist in `docs/testing/multiplayer-online-pilot-guide.md`. Changes remain uncommitted.

## Global Constraints

- Use test-driven development for every behavior change.
- Preserve embedded loopback-only networking and existing HTTPS/WSS enforcement.
- Never log or persist invitation, resumption, or hosted authentication credentials.
- Preserve unrelated dirty work and do not commit or push.

---

### Task 1: Close the replay/live-stream overlap

**Files:**
- Modify: `internal/aphelion/collab/server/websocket.go`
- Test: `internal/aphelion/collab/server/e2e_test.go`

**Interfaces:**
- Consumes: `SessionStore.Load`, `Hub.SubscribeDurable`, authoritative `model.Snapshot.Revision`.
- Produces: replay bounded by `replayHighWater model.Revision`; live queue skips revisions at or below it.

- [ ] Add a blocking-store WebSocket test that commits an operation after durable subscription but before replay finishes and asserts each revision arrives exactly once.
- [ ] Run `go test ./internal/aphelion/collab/server -run TestWebSocketReplayLiveOverlap -count=1` and confirm the duplicate failure.
- [ ] Bound replay to the joined snapshot revision and skip queued durable operations at or below that revision.
- [ ] Run the focused server test and the full server package tests.

### Task 2: Make exact duplicate delivery idempotent

**Files:**
- Modify: `internal/aphelion/collab/client/executor.go`
- Test: `internal/aphelion/collab/client/executor_test.go`

**Interfaces:**
- Consumes: retained accepted-operation map and current acknowledged snapshot hash.
- Produces: `Receive(protocol.ServerEnvelope) error`, returning nil for a byte-equivalent duplicate and an integrity error for altered or unknown stale acceptance.

- [ ] Add tests for identical duplicate, changed duplicate, wrong hash, and skipped revision.
- [ ] Run the focused tests and confirm identical duplicate currently terminates the executor.
- [ ] Add exact duplicate recognition before projection reconciliation and return receive errors to callers.
- [ ] Run all client tests and race-test the package.

### Task 3: Recover from genuine integrity faults

**Files:**
- Modify: `internal/aphelion/collab/ui/session_client.go`
- Test: `internal/aphelion/collab/ui/session_client_test.go`

**Interfaces:**
- Consumes: `NetworkExecutor.Receive(...) error` and existing reconnect/snapshot fallback.
- Produces: transport closure plus `StateReconnecting` after a durable integrity error.

- [ ] Add a session-client test delivering a skipped/altered revision and assert reconnect is initiated.
- [ ] Run it and confirm the current client remains caught up with a terminal executor.
- [ ] On receive failure, record the fault and close the current transport so the existing monitor starts reconnect.
- [ ] Prove snapshot fallback restores caught-up state and does not silently discard pending edits.

### Task 4: Add session-scoped display names

**Files:**
- Modify: `internal/aphelion/collab/protocol/messages.go`
- Modify: `internal/aphelion/collab/server/hub.go`
- Modify: `internal/aphelion/collab/server/http.go`
- Modify: `internal/aphelion/collab/server/websocket.go`
- Modify: `internal/aphelion/collab/ui/session_client.go`
- Modify: `internal/aphelion/collab/ui/panel.go`
- Modify: `internal/aphelion/collab/ui/controller.go`
- Modify: `internal/aphelion/collab/ui/attachment.go`
- Modify: `internal/app/action_user.go`
- Test: corresponding `*_test.go` files.

**Interfaces:**
- Produces: embedded create field `display_name`; `ClientProfileUpdate` with `ProfileUpdatePayload{DisplayName string}`; `SessionClient.UpdateDisplayName(context.Context, string) error`.

- [ ] Add server tests proving an embedded owner name is required/validated and returned through presence.
- [ ] Add hub/protocol tests proving only the authenticated actor can rename itself and invalid names are rejected.
- [ ] Add UI tests for owner name entry and panel rename enablement.
- [ ] Implement the smallest server, client, and UI path satisfying those tests.
- [ ] Add hosted registry persistence tests for session-scoped nicknames without changing issuer/subject identity.
- [ ] Run protocol, server, UI, and app package tests.

### Task 5: Normalize deliberate WebSocket shutdown

**Files:**
- Modify: `internal/aphelion/collab/ui/session_client.go`
- Modify: `internal/app/project.go`
- Test: `internal/aphelion/collab/ui/session_client_test.go`

- [ ] Add tests for normal, already-closed, and unexpected leave errors.
- [ ] Implement a narrow close-error classifier used only during deliberate teardown.
- [ ] Verify unexpected transport errors remain reported.

### Task 6: Full verification and evidence

**Files:**
- Modify: `docs/testing/multiplayer-human-test-guide.md`
- Create: `docs/verification/phase-4-replay-identity-repair-2026-08-26.md`

- [ ] Run `gofmt` on changed Go files.
- [ ] Run focused tests, `go test ./...`, `go test -race ./internal/aphelion/collab/...`, repository lint, and the real Windows StrongDMM build entry point.
- [ ] Run the two-client automated smoke and record exact commands, versions, hashes, and outcomes.
- [ ] Update the human guide with duplicate-replay, recovery, rename, and log-reporting checks.
