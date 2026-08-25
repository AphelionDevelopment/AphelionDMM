# Local Collaboration Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver an authoritative loopback collaboration service that orders durable operations, isolates presence, and proves two-client convergence without external infrastructure.

**Architecture:** Define source OpenAPI/AsyncAPI contracts, place a per-document owner loop around the phase-2 engine, expose bounded HTTP and WebSocket handlers using `coder/websocket`, and launch the same service executable in embedded loopback mode.

**Tech Stack:** Go 1.24, `github.com/coder/websocket`, OpenAPI, AsyncAPI 3, `httptest`, in-memory store.

**Spec:** `docs/superpowers/specs/2026-08-24-multiplayer-design.md`

## Global Constraints

- Read `docs/agent/security-and-networking.md` and `docs/agent/multiplayer-invariants.md`.
- Bind loopback by default and never accept client-supplied paths or commands.
- Durable and presence queues remain separate.
- Do not commit without explicit authorization.
- Do not modify release or CI files without exact approval.

---

### Task 1: Author and validate protocol contracts

**Files:**
- Create: `api/collaboration/openapi.yaml`
- Create: `api/collaboration/asyncapi.yaml`
- Create: `internal/aphelion/collab/protocol/envelope.go`
- Create: `internal/aphelion/collab/protocol/envelope_test.go`
- Create: `internal/aphelion/collab/protocol/testdata/v1/*.json`

- [ ] Write JSON fixture tests for every client/server message and strict rejection of unknown message types, oversized identifiers, missing versions, and invalid operation envelopes.
- [ ] Run `go test ./internal/aphelion/collab/protocol -count=1` and confirm failure.
- [ ] Add OpenAPI paths and AsyncAPI messages exactly as listed in the design spec, with schemas for stable error codes and limits.
- [ ] Implement explicit `ClientEnvelope` and `ServerEnvelope` decoding with `json.Decoder.DisallowUnknownFields` at the envelope layer.

```go
type ClientEnvelope struct {
	ProtocolVersion uint16          `json:"protocol_version"`
	MessageID       string          `json:"message_id"`
	SessionID       string          `json:"session_id"`
	Type            ClientType      `json:"type"`
	Payload         json.RawMessage `json:"payload"`
}
```

- [ ] Validate both contracts using pinned validators and make fixture validation part of the Go test.
- [ ] Regenerate any generated code twice and confirm the second run has no diff.
- [ ] If authorized, commit with `feat(protocol): define collaboration v1 contracts`.

### Task 2: Implement the per-document owner loop

**Files:**
- Create: `internal/aphelion/collab/server/document.go`
- Create: `internal/aphelion/collab/server/document_test.go`
- Create: `internal/aphelion/collab/server/memory_store.go`

- [ ] Write tests that submit concurrent operations and assert one contiguous revision sequence, deterministic accepted order, duplicate idempotency, and no acknowledgement before append success.

```go
type DocumentOwner struct {
	requests chan request
	done     chan struct{}
}

func StartDocument(ctx context.Context, snapshot model.Snapshot, store SessionStore) (*DocumentOwner, error)
func (d *DocumentOwner) Submit(ctx context.Context, op model.Operation) (model.AcceptedOperation, error)
func (d *DocumentOwner) Snapshot(ctx context.Context) (model.Snapshot, error)
func (d *DocumentOwner) Close(ctx context.Context) error
```

- [ ] Run the server test and confirm failure.
- [ ] Implement a single goroutine that exclusively owns the engine. Do not expose engine pointers.
- [ ] Append the normalized accepted operation before returning success or publishing it.
- [ ] Add bounded request queues and context-aware shutdown.
- [ ] Run server tests 100 times and under the race detector.
- [ ] If authorized, commit with `feat(server): serialize authoritative document operations`.

### Task 3: Add session hub and ephemeral presence

**Files:**
- Create: `internal/aphelion/collab/server/hub.go`
- Create: `internal/aphelion/collab/server/presence.go`
- Create: `internal/aphelion/collab/server/hub_test.go`
- Create: `internal/aphelion/collab/server/presence_test.go`

- [ ] Write tests for create/join/leave, viewer/editor/owner authorization, presence coalescing, expiry, slow presence consumer drops, and durable delivery unaffected by presence pressure.
- [ ] Run the tests and confirm failure.
- [ ] Implement immutable `Principal` and `Role` values; derive actor IDs from the authenticated join record.
- [ ] Implement separate durable subscriptions and bounded lossy presence subscriptions.
- [ ] Expire presence on disconnect and configured idle timeout.
- [ ] Run hub/presence tests under race detection.
- [ ] If authorized, commit with `feat(server): add session roles and ephemeral presence`.

### Task 4: Expose bounded HTTP and WebSocket handlers

**Files:**
- Create: `internal/aphelion/collab/server/http.go`
- Create: `internal/aphelion/collab/server/websocket.go`
- Create: `internal/aphelion/collab/server/http_test.go`
- Create: `internal/aphelion/collab/server/websocket_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] Add handler tests for loopback bind policy, launch-token authentication, origin rejection, protocol negotiation, message byte limits, deadlines, join authorization, ping/pong, and graceful close.
- [ ] Run the focused tests and confirm failure.
- [ ] Add a pinned `github.com/coder/websocket` dependency.
- [ ] Implement HTTP routes through `http.ServeMux` and use `http.MaxBytesReader` before decoding.
- [ ] Authenticate and validate origin before WebSocket acceptance. Apply `SetReadLimit` and context deadlines.
- [ ] Route durable and presence message classes to distinct queues.
- [ ] Run focused tests, race tests, and `go test ./... -count=1`.
- [ ] If authorized, commit with `feat(server): expose secure loopback collaboration transport`.

### Task 5: Add embedded service lifecycle and executable

**Files:**
- Create: `internal/aphelion/collab/server/config.go`
- Create: `internal/aphelion/collab/server/embedded.go`
- Create: `internal/aphelion/collab/server/embedded_test.go`
- Create: `cmd/apheliondmm-collab/main.go`

- [ ] Write tests proving zero-value config binds loopback, selects an ephemeral port, creates a single-use token, rejects a second redemption, and drains acknowledged operations on shutdown.

```go
type Embedded struct {
	BaseURL     string
	LaunchToken string
}

func StartEmbedded(ctx context.Context, snapshot model.Snapshot) (*Embedded, error)
func (e *Embedded) Shutdown(ctx context.Context) error
```

- [ ] Run embedded tests and confirm failure.
- [ ] Implement loopback listener creation before serving and return the resolved URL through the trusted in-process result, never command output parsing.
- [ ] Add service flags only for trusted local configuration files and explicit listen mode; reject non-loopback without a complete network configuration.
- [ ] Handle SIGINT/SIGTERM with bounded graceful shutdown.
- [ ] Run the executable against a test fixture and query `/v1/health/live`, `/v1/version`, and a token-authenticated session endpoint.
- [ ] If authorized, commit with `feat(server): add embedded collaboration service lifecycle`.

### Task 6: Prove two-client convergence and reconnect

**Files:**
- Create: `internal/aphelion/collab/server/e2e_test.go`
- Create: `internal/aphelion/collab/server/testclient_test.go`
- Create: `testdata/collaboration/convergence.dmm`

- [ ] Write an end-to-end test starting a real loopback HTTP server and two WebSocket clients.
- [ ] Submit interleaved non-conflicting operations, a conflicting stale operation, and a duplicate operation; assert both clients reach the same revision and canonical hash.
- [ ] Disconnect one client, accept more operations, reconnect from its acknowledged revision, replay, and assert convergence.
- [ ] Flood presence while submitting durable edits and assert no durable acknowledgement is lost or reordered.
- [ ] Run `go test ./internal/aphelion/collab/server -run TestTwoClientsConverge -count=20` and under `-race`.
- [ ] Run all Go tests and `task build`.
- [ ] If authorized, commit with `test(collab): prove loopback two-client convergence`.

## Phase acceptance

- [ ] Source contracts and wire fixtures agree.
- [ ] A single owner produces contiguous authoritative revisions.
- [ ] Acknowledgements follow in-memory durable append and duplicate delivery is idempotent.
- [ ] Presence pressure cannot block durable traffic.
- [ ] Embedded mode binds loopback with a single-use launch token.
- [ ] Two real WebSocket clients converge through conflicts and reconnect.

