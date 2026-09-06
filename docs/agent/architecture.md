# Architecture

## Current application

AphelionDMM retains StrongDMM's desktop and rendering subsystems. The current
editor routes committed map edits through Aphelion's operation engine:

```text
Dear ImGui UI
    -> editor tools
    -> mutable dmmap.Dmm gesture/display state
    -> explicit tile operation -> local or network executor
    -> acknowledged snapshot -> staged, validated DMM/TGM save

Accepted history -> actor-scoped inverse operation for undo
Display refresh -> dmmsnap compatibility copy and render invalidation

Go application
    -> cgo/static library boundary
    -> vendored Rust sdmmparser
```

Important existing seams include:

- `main.go` and `internal/app` for application lifecycle.
- `internal/app/ui/cpwsarea/wsmap/pmap/editor` for map editing and commits.
- `internal/dmapi/dmmap` for the mutable map model.
- `internal/dmapi/dmmsnap` for snapshot-derived undo/redo.
- `internal/dmapi/dmmap/dmmdata` for DMM/TGM parse and write behavior.
- `third_party/sdmmparser` for the Rust parser boundary.

`Editor.CommitOperation` captures explicit before/after tile changes. Network
completion callbacks are scheduled on the UI thread and fenced by attachment
generation. An older acknowledgement does not clear a newer open gesture.
`WsMap.Save` refuses unfinished gestures/submissions, captures the executor's
acknowledged state, and reports staging or replacement failures. Close dialogs
honor that result. The mutable display map is not the save authority.

## Target boundaries

```text
Desktop UI ---- local executor -----+
                                     |
Remote client -- WSS transport ------+--> authoritative operation engine
                                     |        -> validation and ordering
HTTP control/snapshot ---------------+        -> revisioned operation log
                                              -> snapshot store
                                              -> atomic DMM/TGM export

Meridian-MCP <---- versioned adapter / diagnostics coordinator
Content Tools <--- OpenAPI and AsyncAPI contracts
Meridian-Rift <-- staged artifact, hashes, then authoritative build gates
```

New Aphelion-owned packages:

- `internal/aphelion/collab/model`: deterministic domain values and operations.
- `internal/aphelion/collab/engine`: validation, ordering, conflict decisions, and inverse operations.
- `internal/aphelion/collab/protocol`: HTTP/WebSocket wire envelopes and version negotiation.
- `internal/aphelion/collab/server`: sessions, presence, authorization, and transports.
- `internal/aphelion/collab/client`: desktop transport and reconciliation.
- `internal/aphelion/collab/store`: snapshots and operation-log persistence.
- `internal/aphelion/integration`: bounded Meridian and content-tools adapters.
- `cmd/apheliondmm-collab`: the collaboration service executable.

## Dependency rules

- `model` depends only on the Go standard library.
- `engine` depends on `model`, not UI, networking, storage, or ImGui.
- `protocol` maps wire data to `model`; wire compatibility does not leak UI types.
- `server` owns operation ordering and publishes accepted revisions from the document loop after durable append. Stores revalidate retained history and incoming accepted records for consistency.
- `client` never mutates the map outside the same executor abstraction used by local mode.
- inherited UI code depends on narrow Aphelion interfaces; Aphelion packages do not depend on concrete ImGui widgets.
- integrations consume versioned contracts and immutable configuration, not shared database tables.

## Concurrency model

Each open document has one authoritative mutation loop. It serializes durable operations and owns the current revision. Network readers, presence updates, persistence, rendering, and telemetry may run concurrently, but they cannot mutate authoritative document state directly.

Private `SessionStore.LoadRecovery` returns the snapshot, retained operations,
revision hashes, and durable head. Recovery verifies them and restores inverse
targets and historical bases, including operations compacted out of public
reconnect replay. Public `Load` retains its snapshot-plus-suffix shape. This
retains full history and has a measurement-backed scaling investigation in the
2026-09-05 performance audit; no bounded-history optimization is implemented.

The UI thread remains the only owner of OpenGL/ImGui work. Applied operations produce immutable render invalidations that are scheduled onto the UI thread.

## Deployment modes

The shared service package supports these entry points:

- embedded loopback mode in the desktop;
- `cmd/apheliondmm-collab` for local and explicitly enabled LAN service;
- `cmd/apheliondmm-hosted` behind TLS, OIDC, PostgreSQL, backups, and operational monitoring.

Loopback is the default. A deployment mode may strengthen authentication and persistence, but it may not change operation semantics.
