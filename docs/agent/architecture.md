# Architecture

## Current application

AphelionDMM currently inherits StrongDMM's single-process desktop architecture:

```text
Dear ImGui UI
    -> editor tools
    -> mutable dmmap.Dmm
    -> dmmsnap full-map comparison for undo/redo
    -> DMM/TGM writer

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

`Editor.CommitChanges` currently launches asynchronous snapshot comparison after mutations have already occurred. DMM/TGM writers currently write directly to the target path and do not return errors. These are migration constraints, not multiplayer foundations.

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
- `server` owns ordering. Store implementations never decide conflicts.
- `client` never mutates the map outside the same executor abstraction used by local mode.
- inherited UI code depends on narrow Aphelion interfaces; Aphelion packages do not depend on concrete ImGui widgets.
- integrations consume versioned contracts and immutable configuration, not shared database tables.

## Concurrency model

Each open document has one authoritative mutation loop. It serializes durable operations and owns the current revision. Network readers, presence updates, persistence, rendering, and telemetry may run concurrently, but they cannot mutate authoritative document state directly.

The UI thread remains the only owner of OpenGL/ImGui work. Applied operations produce immutable render invalidations that are scheduled onto the UI thread.

## Deployment modes

The same service binary supports:

- embedded loopback mode launched by the desktop client;
- explicitly enabled LAN mode with authentication and TLS rules;
- hosted mode behind TLS, OIDC, PostgreSQL, backups, and operational monitoring.

Loopback is the default. A deployment mode may strengthen authentication and persistence, but it may not change operation semantics.

