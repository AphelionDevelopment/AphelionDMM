# Deterministic Operation Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace after-the-fact mutation discovery with explicit deterministic operations and safe, fidelity-preserving persistence while retaining local single-user behavior.

**Architecture:** Build a UI-independent model and single-owner operation engine under `internal/aphelion/collab`, adapt the mutable StrongDMM map through narrow ports, and route local editing through an executor before adding networking.

**Tech Stack:** Go 1.24, `github.com/google/uuid` UUIDv7 identifiers, existing DMM parser/model, SHA-256 golden fixtures, standard-library atomic file operations.

**Spec:** `docs/superpowers/specs/2026-08-24-multiplayer-design.md`

> **Status reconciliation (2026-08-25):** Automated implementation is complete. This original checklist was not maintained while the work was executed, so its unchecked historical red-test and implementation steps are not current backlog. See `2026-08-25-multiplayer-human-test-readiness.md`; the remaining phase gate is human single-user and two-window desktop regression testing. Changes remain uncommitted by repository policy.

## Global Constraints

- Read `docs/agent/multiplayer-invariants.md`, `docs/agent/architecture.md`, and `docs/agent/generated-and-external-assets.md`.
- Preserve unknown types and variables through every round trip.
- Keep OpenGL and ImGui out of model/engine packages.
- Do not commit without explicit authorization.
- Use narrow `APHELION EDIT` spans in inherited files.

---

### Task 1: Define domain values and canonical hashing

**Files:**
- Create: `internal/aphelion/collab/model/ids.go`
- Create: `internal/aphelion/collab/model/state.go`
- Create: `internal/aphelion/collab/model/operation.go`
- Create: `internal/aphelion/collab/model/hash.go`
- Create: `internal/aphelion/collab/model/hash_test.go`
- Create: `internal/aphelion/collab/model/testdata/canonical-map.json`

- [ ] Write tests proving coordinate order, prefab order preservation, variable-key sorting, length-prefix separation, and repeated hash stability.

```go
func TestSnapshotHashIsCanonical(t *testing.T) {
	left := fixtureSnapshot(map[string]string{"name": `"airlock"`, "dir": "2"})
	right := fixtureSnapshot(map[string]string{"dir": "2", "name": `"airlock"`})
	assert.Equal(t, left.Hash(), right.Hash())
}
```

- [ ] Run `go test ./internal/aphelion/collab/model -run TestSnapshotHashIsCanonical -count=1` and confirm it fails.
- [ ] Implement typed IDs, `Coord`, `PrefabState`, `TileState`, `Snapshot`, `TileChange`, `Operation`, and `AcceptedOperation` matching the approved spec.
- [ ] Implement canonical binary encoding with SHA-256 and explicit errors for invalid dimensions, coordinates, duplicate stable IDs, and malformed hashes.
- [ ] Add JSON golden data and assert the expected digest as a literal compatibility value.
- [ ] Run all model tests twice with `-count=2`.
- [ ] If authorized, commit with `feat(collab): define deterministic operation model`.

### Task 2: Build the authoritative in-memory engine

**Files:**
- Create: `internal/aphelion/collab/engine/document.go`
- Create: `internal/aphelion/collab/engine/errors.go`
- Create: `internal/aphelion/collab/engine/document_test.go`
- Create: `internal/aphelion/collab/engine/conformance_test.go`

- [ ] Write tests for valid apply, stale preconditions, out-of-bounds coordinates, wrong environment hash, wrong document ID, duplicate operation idempotency, and all-or-nothing rejection.

```go
type Document struct {
	snapshot model.Snapshot
	accepted map[model.OperationID]model.AcceptedOperation
}

func NewDocument(snapshot model.Snapshot) (*Document, error)
func (d *Document) Apply(operation model.Operation, acceptedAt time.Time) (model.AcceptedOperation, error)
func (d *Document) Snapshot() model.Snapshot
```

- [ ] Run `go test ./internal/aphelion/collab/engine -run TestDocumentApply -count=1` and confirm it fails.
- [ ] Implement complete validation before mutation and deterministic change ordering.
- [ ] Assign revision `current+1` only after validation. Cache accepted results by operation ID for idempotency.
- [ ] Ensure returned snapshots and operations do not alias mutable engine state.
- [ ] Add a conformance table that can later run against local and network executors.
- [ ] Run `go test ./internal/aphelion/collab/engine -count=1` and `go test -race ./internal/aphelion/collab/...`.
- [ ] If authorized, commit with `feat(collab): add authoritative operation engine`.

### Task 3: Implement actor-scoped inverse operations

**Files:**
- Create: `internal/aphelion/collab/engine/inverse.go`
- Create: `internal/aphelion/collab/engine/inverse_test.go`
- Modify: `internal/aphelion/collab/model/operation.go`

- [ ] Write tests for safe inverse, cross-actor denial, already-inverted target, stale-after-value conflict, and redo as a new forward operation.

```go
func (d *Document) BuildInverse(
	actor model.ActorID,
	target model.OperationID,
	inverseID model.OperationID,
) (model.Operation, error)
```

- [ ] Run the inverse tests and confirm failure.
- [ ] Store sufficient accepted normalized before/after values to build an inverse without reading UI history.
- [ ] Require the requesting actor to match the target actor and require current values to match target after-values.
- [ ] Submit the result through `Apply`; do not add a separate state-rewind path.
- [ ] Run engine tests and the race detector.
- [ ] If authorized, commit with `feat(collab): add actor-scoped inverse operations`.

### Task 4: Add fidelity-preserving map adapters

**Files:**
- Create: `internal/aphelion/collab/mapadapter/import.go`
- Create: `internal/aphelion/collab/mapadapter/export.go`
- Create: `internal/aphelion/collab/mapadapter/roundtrip_test.go`
- Create: `internal/aphelion/collab/mapadapter/testdata/unknown-types.dmm`
- Modify: `internal/dmapi/dmmap/dmm.go`
- Modify: `internal/dmapi/dmmap/dmminstance/instance.go`

- [ ] Write a golden round-trip test containing a known atom, unknown atom path, overridden variables, escaped text, and a multi-z map.
- [ ] Run `go test ./internal/aphelion/collab/mapadapter -run TestUnknownContentRoundTrip -count=1` and confirm failure.
- [ ] Implement import from `dmmap.Dmm` to `model.Snapshot`, assigning UUIDv7 stable IDs in deterministic tile/prefab encounter order only for a newly imported collaboration snapshot.
- [ ] Implement export/apply adapters that preserve opaque path and variable text.
- [ ] Add the minimum marked inherited-file accessors required to preserve unknown prefab data; do not replace the parser or map model.
- [ ] Assert parse -> import -> engine -> export -> parse semantic equality and stable canonical hash.
- [ ] Run adapter, parser, and engine tests.
- [ ] If authorized, commit with `feat(collab): preserve map fidelity through operation adapters`.

### Task 5: Replace direct writers with atomic error-returning APIs

**Files:**
- Modify: `internal/dmapi/dmmap/dmmdata/save_dm.go`
- Modify: `internal/dmapi/dmmap/dmmdata/save_tgm.go`
- Modify: `internal/dmapi/dmmap/dmmdata/dmmdata.go`
- Create: `internal/dmapi/dmmap/dmmdata/save_atomic.go`
- Create: `internal/dmapi/dmmap/dmmdata/save_atomic_test.go`
- Modify: `internal/app/ui/cpwsarea/wsmap/save.go`

- [ ] Write tests using failing writers/validators and a real temporary directory to prove the original target survives every failure and a successful save reparses.

```go
func (d DmmData) WriteDM(writer io.Writer) error
func (d DmmData) WriteTGM(writer io.Writer) error
func SaveAtomic(path string, write func(io.Writer) error, validate func(string) error) error
```

- [ ] Run `go test ./internal/dmapi/dmmap/dmmdata -run TestSaveAtomic -count=1` and confirm failure.
- [ ] Separate serialization from file creation and propagate every write, flush, close, validation, and replacement error.
- [ ] Stage in the destination directory, sync, close, reparse, compare dimensions/hash, and replace atomically using platform-specific helpers where required.
- [ ] Change `WsMap.Save` to return/report the real error and call `ForceBalance` only after success.
- [ ] Add marked edits around inherited writer/UI changes.
- [ ] Run parser/save tests, `go test ./... -count=1`, and the desktop smoke path.
- [ ] If authorized, commit with `fix(map): make saves atomic and observable`.

### Task 6: Introduce the local executor seam

**Files:**
- Create: `internal/aphelion/collab/executor/executor.go`
- Create: `internal/aphelion/collab/executor/local.go`
- Create: `internal/aphelion/collab/executor/local_test.go`
- Modify: `internal/app/ui/cpwsarea/wsmap/pmap/editor/editor.go`
- Modify: `internal/app/ui/cpwsarea/wsmap/pmap/editor/commit.go`
- Modify: concrete tools found by `rg "CommitChanges\(" internal/app`

- [ ] Define and test the executor contract.

```go
type Executor interface {
	Execute(ctx context.Context, operation model.Operation) (model.AcceptedOperation, error)
	BuildInverse(ctx context.Context, target model.OperationID) (model.Operation, error)
	Snapshot(ctx context.Context) (model.Snapshot, error)
}
```

- [ ] Write a local-executor conformance test using the engine suite and confirm it fails.
- [ ] Implement `Local` as a synchronous wrapper around one engine owner. It must not launch mutation goroutines.
- [ ] Add an `Executor` dependency to `Editor` behind a marked narrow adapter.
- [ ] Convert one low-risk tool to build a typed operation before mutation, execute it, then schedule render invalidation on the UI thread.
- [ ] Convert remaining `CommitChanges` callers in small reviewed batches, retaining `DmmSnap` only as a compatibility fallback until every caller is migrated.
- [ ] Delete the asynchronous `CommitChanges` path only after `rg "CommitChanges\("` returns no tool callers and behavior tests cover undo/redo.
- [ ] Run executor/engine tests with race detection, all Go tests, `task build`, and the desktop smoke path.
- [ ] If authorized, commit with `refactor(editor): route local edits through deterministic executor`.

## Phase acceptance

- [ ] Explicit operations, not full-map comparison, account for every migrated edit.
- [ ] Local executor passes the operation conformance suite and race detector.
- [ ] Actor-scoped inverse operations cannot overwrite later conflicting work.
- [ ] Unknown content survives the golden round trip.
- [ ] Failed saves preserve the original file and successful saves reparse/hash-match.
- [ ] The desktop entry point retains single-user edit, undo, redo, and save behavior.
