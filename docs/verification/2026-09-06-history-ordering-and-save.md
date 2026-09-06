# History ordering and saved-state verification

This pass re-inspected command storage, editor acknowledgement callbacks and the
actual workspace Save/close path. It reproduced and repaired two ordering defects
and the command storage's saved-state identity error in the uncommitted tree based
on `052e1acb02b790641d63466de40c91d56028383b`. Previous reports were investigation
pointers, not evidence that these paths were correct.

## Reproductions

An independently accepted edit can arrive before a pending undo/redo callback.
Previously, a normal push immediately changed the stack:

- During undo, the new edit replaced the top entry. The successful inverse then
  failed the original top-ID check, leaving the already-undone command in undo.
- During redo, the new edit cleared redo before the acknowledgement. The forward
  operation was accepted, but its undo command was lost.

`TestAcceptedEditDuringHistoryAcknowledgement` reproduced both failures.
`TestHistoryNewSelectionEditDuringAcknowledgement` reproduced them through real
hidden OpenGL workspaces, Grab nudges, a real network executor and real engine
acceptance. The controlled transport deliberately delivers the independent edit
before the history acknowledgement while maintaining valid revision/hash order.
This is not a live remote-server timing or capacity measurement.

`appliedCommandId` also returned the first undo entry, rather than the latest.
After saving A/B, undoing B and applying C, the stack had the saved depth and first
ID, so `IsModified` returned false. A pending inverse could also appear clean
until its completion. Both fail in dedicated command regressions.

The actual map close path has an independent authority-hash guard. The workspace
regression proved it still reports unsaved content with a deliberately clean
command marker, leaves the saved file intact, and clears after successful Save.
This pass therefore does not claim the command-marker defect bypassed map-close
protection. An initial uniform-map fixture produced identical DMM bytes after
moving interchangeable objects: stable IDs are intentionally not serialized.
The final fixture rotates the moved cell first to give it distinct saved content;
the corrected red test fails only for the false clean command marker.

## Implementation

`commandStack.push` now queues history entries while a history transition is
busy. Durable edits still submit and apply normally; only insertion into the
local undo/redo stack waits. After acceptance or rejection, the original transition
finishes first and queued entries form a new branch in arrival order. The inverse
target, actor preconditions and protocol remain unchanged.

Completion is applied once. A disposed/reopened stack cannot receive the old
transition or its queue. Flush/disposal clear queued references, extending the
previous popped-slot/redo retention repair. An in-flight command and genuinely
queued entries retain the state required for their completion; live history is
not discarded to reduce memory.

Saved-state comparison now uses the latest applied command ID. Busy history is
marked modified. Redo to the exact saved command can become clean; replacement at
the same depth cannot. The workspace's independent snapshot/hash guard and
pending-operation Save refusal remain in place.

## Evidence

Raw logs are in `.artifacts/history-ordering-audit-2026-09-06/`.

| Gate | Observed result |
| --- | --- |
| Command red tests | `command-red.txt` reproduces both successful-transition ordering errors and false clean saved-depth replacement. `pending-dirty-red.txt` reproduces a pending inverse marked clean. |
| Duplicate completion/order | `queue-order-red.txt` reproduces duplicate completion delivery and an obsolete undo entry; the final regression checks one completion and D/C/B undo order. |
| Workspace red tests | `workspace-red.txt` reproduces both history failures. `close-guard-distinct-red.txt` reproduces only the command-marker error with distinct serialized content. |
| Command package | Passed all tests, including acceptance/rejection, queued FIFO insertion, duplicate completion, saved-state return, disposal/reopen, and collection of queued payloads after disposal. |
| Hidden workspace | Passed all history tests. The four accepted/rejected undo/redo cases verify pending Save refusal, expected errors, complete history availability and exact initial-map hash restoration. Actual Save/close uses the rotated/nudged fixture and verifies file contents. |
| Maintained repository gate | `task verify` passed lint (zero issues), contracts, all Go tests, pinned Rust test/fmt/clippy, parser release build and Windows desktop build. Rust has zero unit tests. |
| Affected race suites | Command storage 1.109 s, editor 1.619 s and settings 1.140 s; all passed. |
| Hidden workspace under race | Explicit history, resize, selection and Save suites passed in 6.248 s with `APHELIONDMM_GL_TEST=1`. |
| Produced parser/loopback smoke | Parser open/save/reparse, authenticated two-client collaboration, revisions 1/2, equal client hashes, leave and natural shutdown passed. |

The inherited ImGui C++ `memset` warning remains. Toolchains are Go 1.25.13
Windows/amd64, Rust 1.82.0 GNU, Task 3.53.1, golangci-lint 2.12.2 and GCC 15.2.0.
SHA-256 provenance:

- Desktop: `a5449ffd27d50c9aaec2a02528dd316706d2edb14a4d89e4fa37e2a390aedb42`.
- Smoke command: `9c1ee147eda10c438e7976ded05879c875819e3f3a48638919e03650573736d2`.
- Smoke input/output: `7d044f7165ca68f32d823f463c6810876c91aa13934d4880fd96810469e9c9be`.
- Both smoke clients: `198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa`.

`manifest.json` records working-tree source/document and binary hashes. The smoke
reports revision `undefined`; its binary hash identifies the artifact. It does
not exercise the changed history UI, which is covered by the hidden workspace
gates. Documentation and the manifest were completed after the code gates.

## Performance scope and remaining work

This change repairs correctness and reference lifetime. Queue processing is
proportional to the entries accumulated during the pending transition; it does
not clone a map or the full existing history. The tests do not measure queue
latency, desktop memory, GPU resources or frame rate. Keep the 0/10/100 resize
checkpoint/lifecycle matrix and long-history resource trials in the performance
workplan, including a stalled/disconnected acknowledgement with continued input.

Fresh inspection also confirms the next engine-copy target: the server's
`submit` function clones the document, then `Document.validate` deep-copies the
snapshot again before changing individual tiles. Capture a current matched
baseline before changing ownership or sharing immutable unchanged tile states.
The earlier pre-streaming-hash timings do not establish the current cost.

The next editor audit target is partial selection-capture failure: distinguish
capture owned by the failed gesture from real previously pending edits before
designing recovery. Placement preview, configurable bindings/grid steps,
navigation and reusable stamps remain in the feature workplan. The repo-wide
engine, projection, store/recovery, load/reconnect, parser/FFI/icon, rendering,
search, startup/reload, import/export and lifecycle measurements remain open.

No user commit, push, protected infrastructure change, live PostgreSQL, hosted CI,
Meridian integration or human desktop acceptance occurred in this pass.

Reproduce with Go 1.25.13 and Rust 1.82.0 GNU from the repository root, checking
each native command's exit code:

```powershell
go test ./internal/app/command -count=1 -timeout 60s
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/app/ui/cpwsarea/wsmap -run '^Test(History|Resize|Selection|SaveAcknowledgementBoundaries)' -count=1 -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
go test -race ./internal/app/command ./internal/app/ui/cpwsarea/wsmap/pmap/editor ./internal/app/ui/cpwsarea/wsmap/pmap/psettings -count=1 -timeout 120s
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
New-Item -ItemType Directory -Force .artifacts/history-ordering-audit-2026-09-06 | Out-Null
go build -o .artifacts/history-ordering-audit-2026-09-06/smoke.exe ./cmd/apheliondmm-smoke
& ./.artifacts/history-ordering-audit-2026-09-06/smoke.exe
```
