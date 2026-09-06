# Selection nudges, journal retention and network transitions

This continues the [selection/projection pass](2026-09-06-selection-lifecycle-and-projection.md)
on baseline commit `052e1acb02b790641d63466de40c91d56028383b`. All earlier working
changes were preserved. Current source and new failure reproductions supplied
the evidence; earlier passing gates were not assumed to cover these behaviors.
Changes remain uncommitted and no protected infrastructure entry point changed.

## User-visible changes

- **Alt+Arrow** moves a finished Grab selection one tile. Left/Right/Up/Down
  buttons expose the same action. Each move uses the existing selection preview
  and operation path, preserves stable IDs, replaces visible destination contents
  and supports actor-scoped undo. It does not alter direction or pixel offsets.
- **Shift+Arrow** again pans the camera five times faster. Source inspection found
  this inherited modifier in the camera handler; the previous exact-modifier
  shortcut change had made it unreachable. The real workspace regression
  reproduced zero movement instead of the expected five-times movement. These
  chords are now registered explicitly. Bare arrows retain ordinary camera pan.
- F1 / Help > Keyboard Shortcuts includes the new registered bindings and
  explains selection movement and faster camera pan. Out-of-bounds nudge buttons
  are disabled; the matching shortcut does nothing at the boundary. An unfinished
  selection, wrong visible level or active text field cannot use these actions.

The initial proposal to use Shift+Arrow for nudges was discarded after inspecting
the inherited camera behavior. Alt+Arrow avoids that conflict. User rebinding,
larger configurable grid steps and mirror transforms remain open workplan items.

## Drag journal repair

The previous Move background cache was bounded, but the editor's `pendingChanges`
map still retained every first-touched tile until release. A one-cell drag
regression failed on its second step with three retained journal entries.

The owned Move now invokes a release callback only after restoring a tile that
belongs to neither the original selection nor the current destination. The
editor removes that tile's before-state. Source entries remain, preserving the
unfinished-gesture guards even when the preview returns home. Revisiting a tile
captures its current before-state again.

The regression traverses and revisits a 128-cell map, including a shift of 127
tiles and a return home. It retains at most two journal tiles for a one-cell
selection, refuses Save/resize while open, restores all tile states and IDs
exactly, and creates no history for the round trip. This establishes bounded
retained journal entries, **not** a measured desktop latency or total-heap
improvement. Map snapshots, source copies, renderer resources and other caches
remain separate costs. No timing benchmark was run for this journal change.

## Network and input evidence

The explicit hidden workspace suite constructs real PaneMap/editor/render
objects and uses the actual NetworkExecutor, protocol decoding, projection and
operation engine. A deterministic in-memory transport controls delivery; these
selection-specific cases are not WebSocket service or durability tests.

| Scenario | Verified result |
| --- | --- |
| Rejected rotation while a newer drag is open | The real rejection callback reports the error and retains inspectable conflict intent. It preserves the newer preview and Save guard. Cancellation then applies the acknowledged map; a fresh nudge uses current contents and its acknowledged undo restores the original hash. |
| Remote change to a passed-over tile | Incoming projection waits for the open gesture. Release submits only source/destination changes. The remote edit survives both acceptance and actor-scoped undo. Moved stable identity is preserved. |
| Queued acknowledgement after close/replacement | A newer drag handle is closed. Neither the old completion nor its later preview/cancel can overwrite the replacement snapshot or add undo to its history. |
| Alt+Right followed by Alt+Up | Actual shortcut dispatch moves the selected contents and bounds without panning; left/right Alt variants work. Two undos restore the original canonical hash, and redo preserves identity. |
| Bounds, unrelated modifiers and text | An out-of-bounds nudge and Ctrl+Alt+Arrow create no edit/history. Alt+Arrow in an active input does not move the selection. Focused tool tests also reject missing selections, open gestures, diagonal/multi-tile/Z/zero shifts. |
| Camera navigation | Bare arrow pan remains; Shift+Arrow produces exactly five times its displacement and leaves selection bounds intact. |

The editor permits an embedding app to supply `ReportCollaborationError`; the
test app records these errors so the actual callback can execute without opening
native modal UI. The shipped app still uses the existing native-dialog fallback.

## Final qualification

Raw logs and source/binary hashes are under ignored
`.artifacts/selection-nudge-2026-09-06/`. Go 1.25.13, Rust
`1.82.0-x86_64-pc-windows-gnu` and Task 3.53.1 were selected explicitly.

| Gate | Evidence |
| --- | --- |
| Focused regressions | Journal and nudge tool tests passed after their failing cases. Hidden shortcut tests first reproduced missing Alt+Arrow and broken Shift+Arrow, then passed. |
| Full repository/build | Final `task verify` passed lint/contracts, all Go tests, Rust test/fmt/Clippy, parser release build and Windows desktop build. Rust reports **0 unit tests**. |
| Owned-package race | `go test -race ./internal/aphelion/... -count=1 -timeout 120s` passed. |
| Explicit hidden workspace | `APHELIONDMM_GL_TEST=1` with `^Test(Selection.*|SaveAcknowledgementBoundaries)$` passed, including the preceding rotation and Save behavior. |
| Hidden workspace race | With `APHELIONDMM_GL_TEST=1`, `go test -race ./internal/app/ui/cpwsarea/wsmap -run '^TestSelection' -count=1 -timeout 60s` passed; retained in `workspace-race.txt`. |
| Rebuilt maintained smoke | Fixture open/save/reparse and loopback two-client operations, convergence, leave and natural shutdown passed. Input/output bytes match; accepted revisions are 1 and 2 and both clients reach `198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa`. |

Desktop SHA-256:
`b9118d92cca3577ac3e7c56215358afe0f87f587c86ca040314edfe4db7a0ebf`.
Smoke's report revision is `undefined`; the retained binary manifest supplies
artifact provenance. Normal Windows account access was used for the maintained
lint and hidden-context gates. The inherited ImGui C++ `memset` warning remains.

## Remaining work

Continue the [editor workplan](../superpowers/plans/2026-09-06-editor-qol-workplan.md)
and the full [performance workplan](../superpowers/plans/2026-09-06-admm-performance-followup.md).
Selection geometry/history behavior after a delayed rejection of a rectangular
transform or completed nudge, temporary-tool/tab focus combinations, and broader
hidden-type/larger selection input sweeps remain open. The current tests establish
the scenarios above, not every network/UI interaction.

Fresh static inspection also finds `commitOperation` reading an executor
snapshot before discovering that a captured gesture has no changes. Measure
cancel/no-op release before optimizing that order, retaining failure recovery and
before-state semantics. Continue engine-copy, durable-store, load-harness,
parser/FFI, renderer/cache and repeated lifecycle investigations. Representative
desktop/GPU measurements, PostgreSQL, hosted CI/container acceptance, real external
integrations, production load and human UI acceptance were not rerun here.

The load harness was re-inspected during final qualification. `load/runner.go`
still sends each operation only after every client has observed the previous
one, sends presence afterward, records all-client observation as acknowledgement
latency, and checks a fetched server snapshot rather than applying each client's
event stream. `readLoadEvents` has uncancellable channel sends. These current
sources confirm that the concurrent-load measurement work remains necessary;
the existing green pilot is not service-capacity evidence.
