# Selection capture and cancellation verification

This pass inspected current selection planning, move previews, editor capture,
operation submission and attachment guards in the uncommitted tree based on
`052e1acb02b790641d63466de40c91d56028383b`. Earlier green reports were used to
locate seams; current source and new failure reproductions determined the repair.

## Reproduced defects

Rotation and mirror capture each source/destination before changing any display
tile. A later capture failure previously left earlier successful captures in
`pendingChanges`. Move initialization did the same. No operation had been applied,
but the attachment guard then rejected a validated replacement as an unfinished
edit. The editor regression reproduces all three cases with an invalid stable ID
in a later cell, verifies unchanged display contents, and attempts real validated
reattachment.

A failed destination batch also retained its earlier new backgrounds in an open
move. Retrying that destination could reuse stale contents and restore them over
a later update on cancellation. The move regression starts with a valid preview,
fails the second new destination, changes the first destination, retries and
cancels. It checks that the new contents survive, the previous preview/bounds
stay unchanged on failure, and capture ownership is released.

Cancellation had two related defects. After restoring a faulted drag, it called
the commit path, which refused the latched fault and left the captures pending.
Releasing without explicitly cancelling retained the preview even though it
could not commit. In the healthy case, that same cancellation commit could
instead publish an unrelated pending edit as `Move Grabbed Area`.

Finally, a new destination could already have a separate pending before-state.
`BeginTileChange` treated that as an existing capture, so the drag acquired and
later released bookkeeping it did not own. A separate hidden-workspace red test
reproduced that overlap after an earlier successful preview.

## Repair and boundaries

- Transform preflight tracks and releases only the entries acquired by that call
  if a later capture fails. It leaves the invalid-content fault in place.
- Move initialization releases its successful captures on failure. Destination
  preflight releases only newly acquired backgrounds when the batch fails; the
  prior preview and its backgrounds remain available.
- A move refuses a new destination already owned by a separate pending edit.
  This ordinary refusal does not latch a global fault. The editor can still
  cancel the drag and commit that other edit independently.
- Cancellation restores the move's own backgrounds and releases their captures.
  It does not invoke the operation commit path. A capture-faulted move is also
  cancelled on release and reports the fault.
- Successful release still commits the preview as one operation. Attachment
  changes continue to close old handles without restoring stale display data.

Production changes are limited to `internal/aphelion/editing/move.go` and the
owned editor `selection_move.go` / `selection_transform.go` adapters. The move's
release callback now covers failed preflight and cancellation as well as restored
passed-over cells. Current production and test callers were inspected.

Invalid content still blocks Save until a validated replacement succeeds. Cleanup
does not clear the fault, reset attachment generation, discard unacknowledged
operations or remove unrelated journal entries. Deliberate attachment replacement
retains the existing history-generation rules; this pass does not promise that
old inverse commands remain usable across that transition.

## Evidence

Raw logs are retained in `.artifacts/selection-capture-audit-2026-09-06/`.

| Gate | Evidence |
| --- | --- |
| Transform/initial move red | `editor-red.txt`: rotation, mirror and move all fail validated reattachment because orphan captures remain. Existing unrelated-edit refusal cases retain their commit capability. |
| Destination retry red | `move-red.txt`: a failed batch retains a new destination and later restores its stale background. |
| Workspace red | `workspace-red.txt`: explicit cancellation remains stuck; faulted release also leaves the preview; healthy cancellation commits unrelated work. |
| Pending destination red | `destination-ownership-red.txt`: a preview acquires a tile with a separate pending edit. |
| Focused editing/editor | All tests passed after the initial repair. Subsequent full affected race and repository gates cover the final destination-ownership check. |
| Focused hidden workspace | `workspace-all-green.txt`: all four new top-level cases pass, including explicit/implicit fault cancellation, unrelated pending edits, and pending network acceptance. |
| Affected race suites | Editing 1.064 s, editor 1.646 s, tools 1.139 s and command storage 1.177 s; all passed. |
| Hidden workspace under race | All 29 top-level history, resize, selection and Save tests passed in 7.697 s with `APHELIONDMM_GL_TEST=1`; no hidden-context skip. |
| Maintained repository gate | `task verify` passed lint (zero issues), contracts, all Go tests, pinned Rust test/fmt/clippy, parser release build and Windows desktop build. Rust has zero unit tests. |
| Produced parser/loopback smoke | Parser open/save/reparse, authenticated two-client operations, equal hashes at revisions 1/2, leave and natural shutdown passed. This command does not exercise selection UI. |

The workspace tests use real hidden OpenGL/ImGui contexts, actual mutable maps,
the shared operation engine, command history and actual Save. The network case
uses a real NetworkExecutor and engine with controlled transport delivery. A
previous rotation stays unacknowledged during failed mirror capture, then its
acceptance callback creates history normally. Validated replacement preserves
revision 1, the engine's hash and the rotated direction, and actual Save succeeds.
This establishes the controlled ordering; it is not live remote-server timing.

The lower-level move fixture contains empty tiles during preview. Dmm.Copy
normalizes nil/empty instance slices, so the final comparison copies both sides
before comparing full contents. An initial raw-pointer comparison detected that
representation difference; the retained red log identifies the actual stale
background defect after the comparison was corrected.

The inherited ImGui C++ `memset` warning remains. Toolchains are Go 1.25.13
windows/amd64, Rust 1.82.0 GNU, Task 3.53.1, golangci-lint 2.12.2 and GCC 15.2.0.
SHA-256 provenance:

- Desktop: `66ef23895b7dc15748ce179bc04897def19ec9d66f14f8568a2d17d5b3dfff61`.
- Smoke command: `f6cded859990696720d07db096479377abcabd6d88bd3f56bf07d54f72b37dca`.
- Smoke input/output: `7d044f7165ca68f32d823f463c6810876c91aa13934d4880fd96810469e9c9be`.
- Both smoke clients: `198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa`.

`manifest.json` records relative source, document, binary and evidence paths with
hashes. No Go/Rust source changed after the gates. Documentation and the manifest
were completed afterward. The smoke reports revision `undefined`; its binary hash
identifies it. Its unchanged hash from the engine-copy pass is expected: this
repair changes editing/UI packages outside that command's dependency graph.

## Performance and remaining work

Failed previews no longer retain backgrounds/journal entries acquired by their
failed batch. Cancellation releases the move's captures immediately. New
bookkeeping visits only acquired selection coordinates; it does not scan or copy
the whole map. This is source and regression evidence. There is no new timing,
allocation-rate, process-memory, GPU or service-capacity measurement in this pass.
The temporary acquisition slices add work proportional to newly captured cells;
include successful and repeatedly failed gestures in the selection-size sweep.

The damaged-display recovery UX remains open when real unsent intent exists:
retain or export that intent before any explicit discard/replace action. The
validated attachment seam is now usable after these failed preflights; this pass
does not add a recovery button or automatically replace authoritative/display
state. Placement preview should reuse explicit capture ownership and keep one
gesture separate from another editor's pending changes.

Continue the feature workplan with cancellable paste placement, configurable
bindings/grid steps, navigation and reusable stamps. Continue the complete
performance matrix: engine validation/index/hash/history, projection, durable
stores/recovery, queues/fanout/slow-consumer recovery, real fixture manifests,
parser/FFI/icons, render/GPU, search, startup/reload, import/export and lifecycle.
Live PostgreSQL, hosted CI, Meridian integration and human desktop acceptance
remain separate unrun gates. Changes are uncommitted; protected infrastructure
and external deployments are unchanged.

Reproduce with Go 1.25.13 and Rust 1.82.0 GNU from the repository root, checking
every native exit code:

```powershell
go test ./internal/aphelion/editing ./internal/app/ui/cpwsarea/wsmap/pmap/editor -count=1 -timeout 90s
go test -race ./internal/aphelion/editing ./internal/app/ui/cpwsarea/wsmap/pmap/editor ./internal/app/ui/cpwsarea/wsmap/tools ./internal/app/command -count=1 -timeout 120s
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/app/ui/cpwsarea/wsmap -run '^Test(History|Resize|Selection|SaveAcknowledgementBoundaries)' -count=1 -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
go build -o .artifacts/selection-capture-audit-2026-09-06/smoke.exe ./cmd/apheliondmm-smoke
& ./.artifacts/selection-capture-audit-2026-09-06/smoke.exe
```
