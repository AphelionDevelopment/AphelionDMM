# Selection mirrors and operation authority

Current source inspection and workspace regressions extend the editor tools and
repair two correctness faults. Changes remain uncommitted on
`052e1acb02b790641d63466de40c91d56028383b`, preserving prior working-tree changes.

## Behavior

With a finished Grab selection on the visible level, `H` reflects visible
contents left/right and `V` reflects top/bottom. Matching toolbar buttons and the
live F1 / Help shortcut reference expose both actions. The selection rectangle
stays fixed. Shortcuts require exact modifiers and pause in active text fields.

`internal/aphelion/editing/mirror.go` plans the complete transform before the
editor changes display tiles. It retains stable IDs and unknown types/variables,
keeps hidden instances at their original coordinates, and preserves relative
order within the hidden and visible groups. Hidden destination instances precede
the incoming visible group, matching rotation's merge convention. It swaps the
appropriate cardinal direction bits and negates finite numeric `pixel_x/step_x`
or `pixel_y/step_y`. Unaffected-axis expressions are preserved. Unsupported
direction or affected-offset expressions reject the whole action.

Inherited orientation defaults come from the loaded environment. Redundant
overrides may be removed, and named/numeric orientation text may normalize.
Applying the same mirror twice restores semantic orientation; only undo promises
the exact captured before-state, including the original variable representation.
Sprite matrices, asymmetric art and game-specific connectivity/type swaps are
not inferred. Those require project-specific transform adapters and fixtures.

The editor captures every target before installing any planned content, then
submits through the existing local/network executor. Accepted operations enter
actor-scoped undo history; speculative or rejected operations do not. Tool
bookkeeping uses the existing selection outcome and attachment ownership path.
The planner visits selected cells/instances and caches conversion per shared
prefab within the call. No new performance measurement is claimed here.

## Reproduced defects and repairs

1. **Failed authority selected another commit engine.** A malformed stable ID
   makes the real `InstanceReplace` path fail before-state capture. Previously,
   `CommitOperation` then created a legacy snapshot undo command. A late edit
   after editor close reached the same fallback with no executor. The owned
   `commitWithAuthority` guard now reports the fault and creates no history or
   authority revision. Save remains blocked. The obsolete implementation is
   retained only in a provenance comment. A validated replacement attachment
   clears the capture fault and restores its snapshot, without adopting the
   invalid display edit. Failing evidence: `authority-red.txt`.
2. **An unchanged transform masked earlier selection undo.** After a nudge, a
   no-op horizontal mirror submitted no map operation, but left an enabled
   selection-history node. Undo restored the map while retaining the nudged
   bounds. Both empty commit exits now signal that no transform was applied,
   allowing the immediate provisional node to be discarded. The real workspace
   regression checks unchanged revision followed by exact map hash and bounds
   restoration. Failing evidence: `noop-mirror-reproduced.txt`. The earlier
   `noop-mirror-red.txt` exposed a fixture issue: an explicit inherited-direction
   override first needed normalization before the mirror was actually a no-op.

The failure guard does not restore arbitrary unvalidated display edits or provide
a new recovery UI. A transform capture fault after earlier valid captures may
leave a partial journal, which continues to block Save/attachment replacement.
Recovery of that case remains a separate explicit task; silent adoption or loss
of pending edits is not an acceptable fallback.

## Verification

Raw evidence is under `.artifacts/selection-mirror-2026-09-06/`.

- Focused owned editing/hotkey and editor/tool suites passed. Additional planner
  tests cover rectangular reflection, stable identity, opaque unrelated variables,
  named diagonal directions, affected versus unaffected offsets, hidden/visible
  relative ordering, inherited defaults and whole-action rejection.
- Real hidden-workspace mirror tests passed: H/V shortcuts, modified-key and
  text-input suppression, non-visible level guard, local exact undo/redo, delayed
  network acceptance/rejection and the no-op mirror/nudge undo regression.
  The network case uses the real executor/engine with a controlled transport;
  it is not an end-to-end WebSocket mirror interaction test.
- Final `task verify` passed after the no-op repair: lint (zero issues), contract
  gates, all Go tests, pinned Rust test/fmt/clippy, parser release build and the
  Windows desktop build. The Rust crate reports zero unit tests. The inherited
  ImGui `memset` compiler warning remains.
- Affected race suites passed: owned editing (1.099 s), hotkeys (1.068 s), tools
  (1.142 s) and editor (1.698 s). The explicit hidden workspace selection and Save
  suite passed under race in 5.371 s with `APHELIONDMM_GL_TEST=1`.
- The rebuilt smoke command passed parser open/save/reparse, two authenticated
  loopback clients, revisions 1 and 2, identical client hashes, leave and natural
  shutdown. Its input/output file hashes match. This does not establish human
  desktop interaction or new database/hosted/integration acceptance.

The final logs are `task-verify-final.txt`, `affected-race.txt`,
`workspace-race.txt`, `workspace-mirror-final.txt` and `smoke-report.json`.
Earlier exploratory test outputs are retained separately. The first added
editor integrity test had an incorrect method name and one-cell fixture; those
test setup errors were corrected before the final gates. No production source
changed after final verification. Documentation and the evidence manifest were
completed afterward.

Toolchains: Go 1.25.13 Windows/amd64, Rust 1.82.0 GNU, Task 3.53.1,
golangci-lint 2.12.2 and GCC 15.2.0. SHA-256 provenance:

- Desktop: `30a85a8045474cca11b20ff6bb2584bb1f3a0040c498e715b9d11af2806ac656`.
- Smoke command: `9c1ee147eda10c438e7976ded05879c875819e3f3a48638919e03650573736d2`.
- Smoke input/output: `7d044f7165ca68f32d823f463c6810876c91aa13934d4880fd96810469e9c9be`.
- Both smoke clients: `198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa`.

The smoke report's revision is `undefined`; `manifest.json` records binary and
working-tree source hashes. These tests add correctness and build evidence, not
new measurements for the earlier component performance results.

## Reproduction and next work

Use Go 1.25.13 and Rust 1.82.0 GNU from the repository root:

```powershell
go test ./internal/aphelion/editing ./internal/aphelion/hotkeys ./internal/app/ui/cpwsarea/wsmap/tools ./internal/app/ui/cpwsarea/wsmap/pmap/editor -count=1 -timeout 90s
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/app/ui/cpwsarea/wsmap -run '^Test(Selection|SaveAcknowledgementBoundaries)' -count=1 -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
go build -o .artifacts/selection-mirror-2026-09-06/smoke.exe ./cmd/apheliondmm-smoke
& ./.artifacts/selection-mirror-2026-09-06/smoke.exe
```

Check each native exit code. The smoke executable exercises parser round-trip and
authenticated loopback collaboration; it does not drive desktop mirror controls.

Continue the [editor workplan](../superpowers/plans/2026-09-06-editor-qol-workplan.md)
and [repo-wide performance workplan](../superpowers/plans/2026-09-06-admm-performance-followup.md).
Next correctness work is local resize maintenance and failure/recovery boundaries
around partial captures. Placement preview, configurable bindings/grid steps,
navigation and stamps remain open. Performance work still includes representative
fixtures and desktop/GPU stage timing, engine/projection copies, durable-history
scaling, concurrent load/recovery campaigns, parser/icon lifetime, idle behavior
and repeated lifecycle resource checks. Human tool acceptance, real PostgreSQL,
hosted CI/deployment and Meridian integration gates are distinct and unrun here.

The subsequent [local resize pass](2026-09-06-local-resize-and-history.md) replaces
the separate legacy resize path and records its own fresh verification. The
partial-capture recovery and broader performance/acceptance work above remain open.
