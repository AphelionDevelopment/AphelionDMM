# Cancellable paste placement audit

Baseline: `052e1acb02b790641d63466de40c91d56028383b`, with the existing
uncommitted audit repairs preserved. Current clipboard, editor, tool, input,
operation and history source was inspected directly.

## Findings and implementation

The previous paste path used `InstancesSet` for both retained hidden instances
and copied prefabs, replacing hidden destination identities. It skipped
out-of-bounds cells, committing a clipped selection, and anchored sparse data at
its first sorted tile instead of independent minimum X/Y. The application
immediately committed after `TilePasteSelected`, leaving no cancellation step.
Workspace regressions reproduce the hidden-ID and clipping failures.

Ctrl/Cmd+V now starts placement. Cursor movement updates the display preview;
a canvas click or Enter confirms one operation. Escape, tool switch, map
deactivation and level change cancel; closed attachment handles cannot mutate
again. Toolbar controls and the F1 reference describe the interaction.

`internal/aphelion/editing/placement.go` creates an independent normalized
template with fresh stable IDs once per placement. The existing Move lifecycle
owns current destination backgrounds. Sparse holes stay untouched, passed-over
tiles are restored and released, and hidden destination IDs/values survive.
The whole template must fit on the visible level and contain at most 4096 tiles.
Duplicate cells, mixed source levels and malformed instances are refused before
capture. The clipboard remains unchanged.

Owned editor adapters and narrow inherited spans keep the existing operation
engine, renderer and clipboard. Open placement blocks Save, replacement, resize,
competing commits and history execution, including an invalid initial target
with an empty journal. Instance/tile and variable-edit paths cannot mutate the
template. Repeated Paste keeps the preview. Confirmation selects its destination
rectangle for existing Grab transforms; floating-template rotation/mirroring is
follow-up work. No protocol, persistence format, dependency or asset is added.

The tool retains its originating editor. Cancellation cannot target a later
global editor, commit another edit or reset pending acknowledgement ownership.
Accepted paste enters history; rejection reconciles authority, retains the
conflict and clears the selection when it still belongs to that paste. Local
undo/redo preserves exact map hashes and pasted IDs; controlled network undo
restores the original hash.

## Evidence

Raw results: `.artifacts/paste-preview-audit-2026-09-06/`.

| Gate | Result |
| --- | --- |
| Original workspace red | `workspace-red.txt`: hidden ID replaced; edge paste clipped. |
| New model/API red | `model-red.txt`, `interaction-red.txt`: placement constructor/lifecycle absent. |
| Competing-command red | `mutation-guard-red.txt`: another command changes unfinished paste. Fixed with mutation/commit guards. |
| Canvas focus red | `focus-red.txt`: outside-canvas click confirms placement. Fixed with canvas-activity check. |
| Model and tool checks | Sparse anchors/holes, copied IDs/opaque values, capture rollback, bounds, cancellation, originating editor, unchanged position and tool frame clicks pass. |
| Hidden keyboard/Save | Registry dispatch handles modified Enter, active text, and map Enter; real workspace Save succeeds after confirmation. |
| Affected race | Editing, tools, pane/editor/settings, variable editor and command suites pass. Packages with no tests retain that limit. |
| Complete hidden workspace race | 36 top-level tests pass in 18.193 s, no skips: existing history/resize/selection/Save and new paste cases. |
| Repository gate | `task-verify-green.txt`: lint zero issues, contracts, all Go tests, pinned Rust test/fmt/clippy, parser release and Windows desktop build pass. Rust has zero unit tests. |
| Final build | `build-final.txt`: desktop rebuilt after a provenance-only import comment. No behavior changed after green gates. |
| Produced parser/loopback smoke | Open/save/reparse, two authenticated clients, revisions 1/2, identical hashes, leave and shutdown pass. This command does not exercise paste UI. |

The first broad run exposed a variable-editor guard-order regression introduced
here: querying the editor before the existing unknown-type read-only check.
The original check now runs first. Its race test and the subsequent repository
gate pass. One keyboard fixture sent Escape after its text window closed,
correctly cancelling paste; the corrected fixture omits that extra cancellation.

Hidden OpenGL tests require the normal Windows account because the editor's
profile lookup fails in the isolated runner account. The normal-account run
passed. The inherited ImGui C++ `memset` warning remains.

Toolchain: Go 1.25.13 windows/amd64, Rust 1.82.0 GNU, Task 3.53.1,
golangci-lint 2.12.2, GCC 15.2.0. SHA-256:

- Desktop: `ba586c9afe499b537486563e9fb8b8e26e56222c4f8ecff7cf64d06808d5f420`.
- Smoke: `f6cded859990696720d07db096479377abcabd6d88bd3f56bf07d54f72b37dca`.
- Smoke input/output: `7d044f7165ca68f32d823f463c6810876c91aa13934d4880fd96810469e9c9be`.
- Both smoke clients: `198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa`.

`manifest.json` records relative paths/content hashes. The smoke revision field
is `undefined`; its binary hash identifies it. Its unchanged hash is expected:
paste UI packages are outside that command's dependency graph.

## Performance and remaining gates

`TestPlacementUnchangedPositionDoesNoWork` measures zero allocations per repeated
model Preview at the same position over 100 allocation runs; capture and
regeneration counters remain at one. A two-cell sparse traversal retains exactly
two backgrounds after each successful preview, restoring passed-over cells and
holes. These are component/bookkeeping results, not desktop latency, GPU,
process-memory or throughput measurements.

The [application routing follow-up](2026-09-06-canvas-resize-and-application-routing.md)
now dynamically qualifies the actual Ctrl+V dispatcher and rendered workspace
focus transitions. That previously unrun gate is closed by its scoped evidence.

Still required:

- Human acceptance of toolbar focus, click/Enter/Escape, keyboard layouts, tab
  focus and continued rotation/mirror/nudge after placement.
- Expanded malformed-instance, all-level, live pending-acknowledgement and
  attachment-replacement interactions beyond the bounded cases above.
- Representative clipboard sizes, sparse bounding rectangles, render bucket
  invalidation, input-to-visible latency, allocation rate and lifecycle resources.
- Floating-template transforms, rebinding/grid steps, safe recovery UI,
  navigation/selection tools and stamps in the feature workplan.

The complete repo-wide performance matrix remains open. Live PostgreSQL, hosted
CI/deployment and Meridian integration are separate unrun gates. All changes
are uncommitted; protected infrastructure is unchanged.

Reproduce from the repository root with the pinned toolchains, checking each
native exit code:

```powershell
go test -race ./internal/aphelion/editing ./internal/app/ui/cpwsarea/wsmap/tools ./internal/app/ui/cpwsarea/wsmap/pmap/... ./internal/app/ui/cpvareditor ./internal/app/command -count=1 -timeout 120s
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/app/ui/cpwsarea/wsmap -count=1 -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
go build -o .artifacts/paste-preview-audit-2026-09-06/smoke.exe ./cmd/apheliondmm-smoke
& ./.artifacts/paste-preview-audit-2026-09-06/smoke.exe
```
