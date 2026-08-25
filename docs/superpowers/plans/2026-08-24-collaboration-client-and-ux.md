# Collaboration Client and UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the desktop editor a complete collaboration client with responsive speculative editing, authoritative reconciliation, accessible session state, presence, reconnect, conflicts, and actor-scoped undo.

**Architecture:** Keep transport and reconciliation in Aphelion-owned packages, expose a testable view model to a thin ImGui panel, and route every map tool through the phase-2 executor contract. UI-thread rendering consumes immutable authoritative/speculative projections.

**Tech Stack:** Go 1.24, `coder/websocket`, Dear ImGui, existing OpenGL renderer, phase-3 OpenAPI/AsyncAPI contracts.

**Spec:** `docs/superpowers/specs/2026-08-24-multiplayer-design.md`

## Global Constraints

- Read `docs/agent/architecture.md`, `docs/agent/multiplayer-invariants.md`, and `docs/agent/generated-and-external-assets.md`.
- Do not create collaboration icons, colors, sounds, branding, or prose assets. Use existing neutral primitives and text.
- Keep ImGui/OpenGL calls on the UI thread.
- Keep credentials and launch tokens out of logs, URLs, and persisted layout state.
- Do not commit without explicit authorization.

---

### Task 1: Implement the protocol transport and connection state machine

**Files:**
- Create: `internal/aphelion/collab/client/transport.go`
- Create: `internal/aphelion/collab/client/websocket.go`
- Create: `internal/aphelion/collab/client/state.go`
- Create: `internal/aphelion/collab/client/state_test.go`

- [x] Write transition tests for disconnected, connecting, synchronizing, caught-up, reconnecting, read-only, conflict, and closed states.

```go
type State string

const (
	StateDisconnected  State = "disconnected"
	StateConnecting    State = "connecting"
	StateSynchronizing State = "synchronizing"
	StateCaughtUp      State = "caught_up"
	StateReconnecting  State = "reconnecting"
	StateReadOnly      State = "read_only"
	StateConflict      State = "conflict"
	StateClosed        State = "closed"
)

type Transport interface {
	Connect(ctx context.Context, request protocol.JoinRequest, receive func(protocol.ServerEnvelope)) error
	Send(ctx context.Context, message protocol.ClientEnvelope) error
	Close(status websocket.StatusCode, reason string) error
}
```

- [x] Run `go test ./internal/aphelion/collab/client -run TestConnectionTransitions -count=1` and confirm failure.
- [x] Implement state transitions as a reducer with invalid-transition errors.
- [x] Implement WebSocket transport with one reader, one writer, separate bounded durable/presence outbound queues, and context cancellation.
- [x] Coalesce presence and never drop durable submissions locally.
- [x] Test server close codes, malformed envelopes, deadline expiry, and cancellation under race detection.
- [ ] If authorized, commit with `feat(client): add collaboration transport state machine`.

### Task 2: Add a network executor with speculation and reconciliation

**Files:**
- Create: `internal/aphelion/collab/client/executor.go`
- Create: `internal/aphelion/collab/client/reconcile.go`
- Create: `internal/aphelion/collab/client/reconcile_test.go`
- Modify: `internal/aphelion/collab/executor/executor.go`

- [x] Write tests for speculative apply, authoritative acceptance, rejection rollback, out-of-order server-message refusal, reapplication of compatible pending edits, and hash mismatch fail-closed behavior.

```go
type Projection struct {
	Acknowledged model.Snapshot
	Pending      []model.Operation
}

func (p Projection) Submit(operation model.Operation) (Projection, error)
func (p Projection) Accept(accepted model.AcceptedOperation) (Projection, error)
func (p Projection) Reject(rejected protocol.OperationRejected) (Projection, Conflict, error)
```

- [x] Run reconciliation tests and confirm failure.
- [x] Implement a pure projection reducer; do not mutate `dmmap.Dmm` inside it.
- [x] Implement the network `Executor` by assigning operation IDs, submitting, awaiting the matching result, and exposing projection updates.
- [x] Apply immutable projection updates to the map adapter on the UI thread.
- [x] Run the phase-2 executor conformance suite against both local and network executors.
- [ ] If authorized, commit with `feat(client): reconcile speculative edits to server revisions`.

### Task 3: Add desktop session lifecycle control

**Files:**
- Create: `internal/aphelion/collab/ui/controller.go`
- Create: `internal/aphelion/collab/ui/controller_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/action_user.go`
- Modify: `internal/app/project.go`

- [x] Write controller tests for create embedded session, join with scoped token, leave, project-close confirmation, server shutdown, and token redaction.

```go
type Controller struct {
	service  EmbeddedService
	client   CollaborationClient
	view     ViewModel
}

func (c *Controller) CreateLocal(ctx context.Context, snapshot model.Snapshot) error
func (c *Controller) Join(ctx context.Context, invitation Invitation) error
func (c *Controller) Leave(ctx context.Context) error
```

- [x] Run controller tests and confirm failure.
- [x] Implement lifecycle orchestration without giving the controller direct map mutation access.
- [x] Add narrow marked initialization/shutdown calls to `internal/app/app.go`.
- [x] Prevent environment/project replacement while a session has unacknowledged operations; revalidate after user confirmation returns.
- [x] Clear launch/join tokens immediately after redemption and exclude them from config serialization.
- [ ] Run focused tests and the desktop close/reopen smoke path.
- [ ] If authorized, commit with `feat(ui): manage desktop collaboration sessions`.

### Task 4: Add the collaboration panel and menu actions

**Files:**
- Create: `internal/aphelion/collab/ui/viewmodel.go`
- Create: `internal/aphelion/collab/ui/viewmodel_test.go`
- Create: `internal/aphelion/collab/ui/panel.go`
- Modify: `internal/app/ui/layout/layout.go`
- Modify: `internal/app/ui/layout/lnode/lnode.go`
- Modify: `internal/app/ui/menu/menu.go`

- [x] Write view-model tests for status labels, role capabilities, participant ordering, conflict summaries, copy-invite availability, and accessible error text.
- [x] Run focused tests and confirm failure.
- [x] Implement a pure `ViewModel` with no ImGui dependency.
- [ ] Render an Aphelion-owned panel showing session name/ID, role, revision, sync state, participants, invite, leave, reconnect, and bounded conflict details.
- [x] Add marked layout/menu registration. Bump layout state only if required and document the resulting persisted-layout reset before editing.
- [x] Use existing neutral UI styling and text. Do not add or alter assets.
- [ ] Exercise keyboard access, status readability without color, narrow panel width, and screen-reader-compatible status logging where the desktop framework permits it.
- [ ] If authorized, commit with `feat(ui): add collaboration session panel`.

### Task 5: Render collaborator presence safely

**Files:**
- Create: `internal/aphelion/collab/ui/presence.go`
- Create: `internal/aphelion/collab/ui/presence_test.go`
- Modify: `internal/app/ui/cpwsarea/wsmap/pmap/overlay/overlay.go`
- Modify: `internal/app/ui/cpwsarea/wsmap/pmap/editor/overlay.go`

- [x] Write tests for coordinate conversion, off-level suppression, expiry, label fallback, participant cap, and no durable map changes.
- [x] Run presence tests and confirm failure.
- [x] Convert server presence into immutable overlay commands on the UI thread.
- [x] Reuse existing overlay primitives with text labels and deterministic neutral style slots. Do not create new visual assets.
- [x] Coalesce local cursor updates to the server-negotiated presence rate.
- [x] Extend protocol v1 presence with bounded selection data, then publish selection updates at the negotiated rate.
- [x] Confirm presence disappears on timeout/disconnect and never changes save output or canonical hash.
- [ ] Run focused, race, and desktop visual smoke checks.
- [ ] If authorized, commit with `feat(ui): display ephemeral collaborator presence`.

### Task 6: Complete conflict, reconnect, and actor undo UX

**Files:**
- Create: `internal/aphelion/collab/client/reconnect.go`
- Create: `internal/aphelion/collab/client/reconnect_test.go`
- Create: `internal/aphelion/collab/ui/conflict.go`
- Create: `internal/aphelion/collab/ui/conflict_test.go`
- Modify: `internal/app/command/storage.go`
- Modify: `internal/app/action_user.go`

- [x] Write tests for exponential reconnect with jitter bounds, resume from acknowledged revision, snapshot fallback, inverse success, inverse conflict, and no shared-history rewind.
- [x] Run tests and confirm failure.
- [x] Implement bounded reconnect delays that stop on explicit leave, authentication denial, or incompatible protocol.
- [x] Present conflict values and actions: refresh authoritative state, discard local edit, or rebuild a new operation from current state. Never offer force overwrite as ordinary conflict resolution.
- [x] Adapt desktop undo/redo actions to call actor inverse/forward operations when collaboration is active; move history only after acknowledgement and preserve synchronous local command behavior.
- [ ] Revalidate session/revision after any modal input resolves.
- [x] Run end-to-end tests with forced disconnect during pending operations.
- [ ] If authorized, commit with `feat(ui): handle collaboration reconnect and safe undo`.

### Task 7: Exercise the shipped desktop multiplayer path

**Files:**
- Modify: `cmd/apheliondmm-smoke/main.go`
- Modify: `internal/aphelion/smoke/report.go`
- Create: `testdata/collaboration/desktop-session.json`

- [x] Extend smoke-report tests with service launch, two desktop client identities, operation revisions, final hashes, leave, and clean shutdown.
- [ ] Run the smoke tests and confirm failure.
- [ ] Add automation hooks that drive the real built executable without bypassing session lifecycle code.
- [x] Build `dst/StrongDMM.exe` and `cmd/apheliondmm-collab` with pinned toolchains.
- [ ] Open the fixture, start a local session, join a second client, edit from both, exercise conflict/undo/reconnect, save, reparse, and close both processes.
- [ ] Record the exact report and any platform not exercised.
- [ ] If authorized, commit with `test(ui): exercise desktop multiplayer entry point`.

## Phase acceptance

- [x] Local and network executors pass the same conformance suite.
- [ ] All map tools submit explicit operations when collaboration is active.
- [x] Speculation reconciles without declaring local state authoritative.
- [ ] Collaboration state and conflicts remain understandable without color or new assets.
- [x] Presence is lossy, bounded, and absent from saves/hashes.
- [x] Reconnect and actor undo preserve convergence.
- [ ] The real desktop path completes a two-client session and atomic save.
