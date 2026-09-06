# ADMM code audit — 2026-09-05

Reviewed checkout: `18429fc79bf69bb4720cfc92c13d028313438024`. The working tree was clean at the start. This audit changes documentation and adds overlay probe inputs only; application source, existing tests, infrastructure, and Git history are unchanged.

The review found **nine actionable defects: seven P1 and two P2**. Ten targeted probes reproduced eight of the defects; the remaining Save finding is established by the shipped source path. Existing tests passing does not establish these missing interleavings and recovery cases. Repair durability, ordering, and desktop state handling before relying on collaborative editing for durable work.

P1 means a correctness or data-integrity issue to resolve before multiplayer acceptance. P2 means a narrower protocol correctness issue requiring repair. These priorities do not claim that every deployment encounters every trigger.

## Findings

### F1 — P1: snapshot compaction discards validation context and can strand acknowledged edits

Sources: `internal/aphelion/collab/engine/document.go:21`, `engine/inverse.go:19`, `server/recovery.go:84`, `store/sqlite/store.go:156`, `store/sqlite/store.go:508`, `store/postgres/store.go:500` (all shortened paths in this paragraph are below `internal/aphelion/collab/`).

`engine.NewDocument` starts with only the snapshot's revision hash and empty accepted/inverted operation maps. The SQL stores load only operations newer than the saved snapshot, then build a fresh engine to validate appends. That loses the older base hashes and inverse targets which the live document still accepts.

Three reproduced cases using a real temporary SQLite store:

1. Accept edit A, save its snapshot, build a safe inverse of A in the still-running owner, and submit it: the store rejects it with `operation_not_found`.
2. Accept A at revision 1, save its snapshot, then submit a nonoverlapping edit based on authentic revision 0: the store rejects it with `unknown_base_revision`.
3. Accept A at revision 1 and B at revision 2, both based on revision 0. Publish the captured revision-1 snapshot after B is acknowledged, as the asynchronous snapshot worker can do. Recovery now fails replaying B with `unknown_base_revision`. This converts an acknowledged, previously valid log into an unrecoverable document through normal snapshotting.

PostgreSQL has the same snapshot-plus-suffix reconstruction in source; the reproduction used SQLite, not live PostgreSQL. Hosted startup enables snapshots after 1,000 operations or five minutes (`cmd/apheliondmm-hosted/main.go:102`); the standalone collaboration command makes snapshotting configurable.

**Required result:** snapshotting must preserve the context needed for authentic stale bases, actor-scoped inverse validation, duplicate results, and recovery. Do not repair this by skipping operation validation or silently discarding acknowledged suffix records.

### F2 — P1: recovery does not compare recovered state with stored integrity metadata

Sources: `internal/aphelion/collab/store/sqlite/store.go:508`, `internal/aphelion/collab/store/postgres/store.go:215`, `internal/aphelion/collab/store/postgres/store.go:500`, `internal/aphelion/collab/server/recovery.go:68`.

The loaders omit persisted `snapshot_hash` from their queries. Recovery hashes a structurally valid snapshot but never compares that hash with the stored snapshot or durable head hash. PostgreSQL's public `Load` also discards the separately stored current revision returned by its internal loader. Replaying an internally consistent suffix alone does not prove that the durable head is intact.

A probe changed only the serialized SQLite snapshot's `max_x` from 3 to 4, leaving the stored hash unchanged. `RecoverDocument` succeeded and served the changed dimensions. The probe deliberately simulated storage corruption; it does not claim a remote database-write exploit.

**Required result:** take a consistent read of snapshot, identity/revision metadata, operation suffix, and durable head. Verify their identities, revisions, continuity, and canonical hashes before making the owner available. Retain the original database on failure.

### F3 — P1: document revisions are assigned in order but broadcast outside that order

Sources: `internal/aphelion/collab/server/document.go:188`, `internal/aphelion/collab/server/hub.go:188`, `internal/aphelion/collab/server/hub.go:224`, `internal/aphelion/collab/server/hub.go:312`, `internal/aphelion/collab/server/websocket.go:250`.

The owner sends a buffered response to each submitting caller. Each caller then independently calls `hub.publishDurable`. The hub mutex protects subscribers but does not preserve document revision order across those callers. The client correctly requires contiguous accepted revisions.

The concurrent probe submitted 128 valid operations to one hub and inspected subscriber arrival order without sorting. Its first round delivered revision **4 while revision 1 was expected**. This causes client suspension/reconnect despite a valid ordered store log. This reproduction exercised the owner/hub subscription, not a deployed WebSocket service.

The same WebSocket writer also emits rejection results directly from `handleClientMessage`, while earlier accepted operations can still be waiting in the durable queue. Repair acceptance ordering and add a deterministic rejection-order test as part of the same work; rejection overtaking was established as a possible source interleaving, not separately reproduced here.

**Required result:** one document-owned publication sequence following durable append, with ordered per-connection delivery and a barrier ensuring rejection authority does not overtake the accepted revisions it references. Include inverse submissions and reconnect handover.

### F4 — P1: normal competing edits discard the rejected user's draft

Sources: `internal/aphelion/collab/client/reconcile.go:76`, `internal/aphelion/collab/client/reconcile.go:189`, `internal/aphelion/collab/client/executor.go:209`, `internal/aphelion/collab/client/executor.go:401`.

When a remote accepted edit changes the same tile as local speculation, `Projection.Accept` first applies the remote edit successfully, then fails while reapplying the local draft. `NetworkExecutor.Receive` treats that normal conflict as a synchronization failure and clears all pending operations. It does not advance the acknowledged snapshot or create a conflict draft.

The probe submitted a local edit, then delivered a valid revision-1 competing edit. Result: `precondition failed`, acknowledged revision **0**, pending **false**, conflicts **0**. The refresh/discard/rebuild UI cannot recover a draft which was never retained.

**Required result:** accept valid authoritative progression even when speculation cannot be reapplied. Preserve incompatible local intent until its operation rejection is correlated and presented as a conflict. A conflict must not be treated as corrupt authoritative history.

### F5 — P1: desktop Save writes speculative map state and marks it saved

Sources: `internal/app/ui/cpwsarea/wsmap/save.go:11`, `internal/app/ui/cpwsarea/wsmap/save.go:27`, `internal/app/ui/cpwsarea/wsmap/save.go:36`, `internal/dmapi/dmmsave/save_process.go:28`, `internal/app/ui/cpwsarea/wsmap/pmap/editor/collaboration.go:148`.

`WsMap.Save` passes `paneMap.Dmm()` directly to `dmmsave.Save`, then balances the command stack. That DMM is also the visible projection and includes local tool mutations and pending network speculation. The save path neither obtains the acknowledged executor snapshot nor checks for an active gesture or unacknowledged submission. Atomic serialization verifies the map it was handed, so it cannot establish that the server accepted those contents.

Trigger: make a network edit, save before its acknowledgement, then have the server reject the edit. The file can contain the rejected change even though the authoritative document does not. This finding is a complete source-path trace; no interactive desktop Save reproduction was run. The workspace package currently has no tests.

**Required result:** save an explicitly captured acknowledged snapshot, or refuse saving while a gesture/submission is unresolved. Preserve the old target and dirty state for blocked or failed saves. Exercise `WsMap.Save` and the Save All/close callers, not just `SaveAtomic`.

### F6 — P1: an earlier acknowledgement erases the next active gesture

Sources: `internal/app/ui/cpwsarea/wsmap/pmap/editor/collaboration.go:273`, `:278`, `:390`, `:425`.

The delayed completion callback calls `syncFromExecutor`, which calls `setAuthoritative`, which unconditionally replaces `pendingChanges` with an empty map. `ProcessCollaborationUpdates` guards against active gestures; the completion callback does not.

The editor probe changed direction 2 → 4 and submitted it, began a second gesture changing 4 → 8, then delivered the first acknowledgement before committing the second gesture. The second gesture's captured before-values disappeared. Its subsequent commit returns immediately because there are no captured changes.

Also review executor lifecycle fencing in this repair: `syncFromExecutor` assigns the captured executor back to `e.executor`; a completion queued before detach can therefore refer to an obsolete attachment. The active-gesture loss was reproduced; stale-attachment rebinding remains a regression scenario to add.

**Required result:** completion updates only the state owned by its submission and attachment generation, preserving any newer gesture and rejecting stale attachment callbacks.

### F7 — P2: retrying an accepted unsorted operation is rejected

Sources: `internal/aphelion/collab/engine/document.go:132`, `internal/aphelion/collab/server/document.go:278`.

The engine sorts the change list for the accepted record. The owner's duplicate path compares that stored normalized operation with the original incoming operation using `reflect.DeepEqual`. An operation sent with coordinates in descending order is accepted the first time and rejected on an identical retry. The probe reproduced `conflicts with stored revision 1`.

**Required result:** use the same deterministic normalization for first delivery and duplicate identity comparison, returning the original accepted result. Continue rejecting reuse of an operation ID with different meaningful contents or actor identity.

### F8 — P2: an exact duplicate accepted event suspends a client after it advances

Source: `internal/aphelion/collab/client/executor.go:394`.

The exact-duplicate fast path only accepts a duplicate when its revision equals the client's current acknowledged revision. After receiving revisions 1 and 2, an identical replay of revision 1 falls through to `Projection.Accept` and fails with `accepted revision is 1, want 3`. The probe reproduced that failure.

**Required result:** retain sufficient identity and hash evidence to ignore exact older accepted duplicates without advancing state, while rejecting altered records or hashes. Do not solve this by ignoring every older revision.

### F9 — P1: opening a preserved unknown prefab crashes the variable editor

Sources: `internal/app/action_user.go:612`, `internal/app/ui/cpvareditor/vareditor.go:119`, `internal/app/ui/cpvareditor/collect.go:41`, `internal/app/ui/cpvareditor/vareditor.go:211`.

Map loading and collaboration intentionally preserve types absent from the environment, but `VarEditor.setup` looks up such a type and passes the resulting nil object to `collectVariablesPaths`. That function immediately reads `obj.Path`. The ordinary Edit Instance and Edit Prefab entry points both reach this setup. The default-value and read-only helpers also assume the environment object exists.

A probe called the real `VarEditor.EditPrefab` with a valid unknown prefab containing an opaque variable and an environment without that type. It reproduced `runtime error: invalid memory address or nil pointer dereference`. This was a headless component reproduction; no interactive desktop was launched.

**Required result:** unknown prefab paths and explicit variables remain inspectable without dereferencing unavailable environment metadata. Either support safe opaque-variable editing through the operation engine or expose an explicit read-only state until metadata is available; never discard the prefab or its variables to avoid the crash.

## Verification evidence

Tool versions read in this audit: Go `1.25.13 windows/amd64`, Task `3.53.1`, GCC `15.2.0`, golangci-lint `2.12.2`. Rust `1.82.0-x86_64-pc-windows-gnu` is installed; the Rust test/build gates were not run. No prior report was reused as current verification.

| Check | Current evidence |
| --- | --- |
| Existing engine/client/server/store Go suites | Passed freshly with `-count=1 -timeout 120s`; PostgreSQL package success does not imply a configured database test ran. |
| Audit probes | Ten probes failed against unchanged application source, reproducing F1, F2, F3, F4, F6, F7, F8, F9. These are expected red regressions, not successful application checks. See the [recorded observations](audit-2026-09-05/observations.md). |
| First full Go test attempt | Failed: sandbox access denied in Meridian fixture path resolution, plus missing shared compiler-cache artifacts in several packages. This is not a clean full-suite result. |
| Full Go retry with normal filesystem access and isolated cache | **Passed**, exit 0: `go test ./... -count=1 -timeout 120s -p 1`, using the ignored repository-local Go cache. The overlay probes were not installed in this baseline run. |
| Shipped desktop interaction, production cross-stack build | Not run. Editor probe uses the current editor implementation with existing test fixtures. |
| Race, Rust, lint, vulnerability scans | Not run in this audit. Reading the linter version is not a lint result. |
| Live PostgreSQL, container lifecycle, external OIDC/OTLP, public load/fault, hosted CI | Not run. No production-readiness conclusion. |
| Real Meridian-MCP/Rift and Content Tools acceptance | Not run. No game-repository files changed. |

The full Go run emitted a compiler warning from the inherited `imgui-go` C++ dependency (`ImFontAtlasBuildPackCustomRects`, `-Wstringop-overflow`) but completed successfully. This audit does not classify that warning as a demonstrated defect. The existing native parser library was linked without a fresh pinned Rust build, so the Go result is not cross-stack reproducibility evidence. The fixture/access and compiler-cache failures in the first attempt cleared on the isolated-cache rerun with normal filesystem access.

Reproduce the new regressions from the repository root in PowerShell:

```powershell
$env:GOCACHE = Join-Path $PWD '.artifacts/audit-2026-09-05/go-cache'
pwsh -NoProfile -File docs/verification/audit-2026-09-05/run-probes.ps1
$LASTEXITCODE # Expected 1 on the audited revision.
pwsh -NoProfile -File docs/verification/audit-2026-09-05/run-probes.ps1 -Editor
$LASTEXITCODE # Expected 1 on the audited revision.
pwsh -NoProfile -File docs/verification/audit-2026-09-05/run-probes.ps1 -Unknown
$LASTEXITCODE # Expected 1 on the audited revision.
```

The runner maps `.go.txt` probe inputs into virtual `_test.go` files with `go test -overlay`. It removes only its own temporary overlay file and empty temporary directory. It does not copy probes into the source tree, change existing tests, or modify production data. The hub probe is scheduler-sensitive and bounded to ten rounds; the other probes use controlled sequences. Native-linked editor tests require the local parser library and C compiler. Fixture canonicalization may require normal filesystem access outside the restricted execution sandbox.

## Coverage and limits

This was a risk-directed audit, not an assertion that every line or every input is correct. Current source was read in these areas:

| Area | Inspected boundary |
| --- | --- |
| Operation core | Snapshot/ID/hash validation, apply normalization, inverse preconditions, cloning, local and network executors |
| Persistence | Owner append/reconciliation/snapshot loop, recovery, memory and SQLite implementation, PostgreSQL append/load/snapshot and migration paths |
| Network/security | HTTP lifecycle and checkpoint path, WebSocket authentication/origin/limits, role derivation, hosted reauthorization, OIDC manager/flow, rate limiting and proxy handling |
| Desktop | Mutation capture, async acknowledgement, projection updates, attachment, undo/redo, real Save path and serializer |
| Fidelity/integration | Import/export/environment hashing, atomic reparse/semantic validation, immutable staging, fixed MCP and PowerShell acceptance coordinator |
| Delivery | Current Task/CI definitions, skip gates, updater signature boundary, Go/Rust parser bridge |

Additional qualification work is necessary: representative large-map latency/allocation profiles, history retention costs, oversized brush batching, broader inherited-editor mutation coverage, and real integration/deployment acceptance. The source copies full snapshots and retained accepted history during submission, and SQL append reconstructs a document from retained replay; these are performance candidates, not measured regressions or speedup claims. No dependency-advisory audit or upstream reconciliation was performed.

The workplan is [2026-09-05-admm-audit-workplan.md](../superpowers/plans/2026-09-05-admm-audit-workplan.md).
