# Local resize, history and allocation audit

Fresh inspection and failing tests found three resize failures: unsuccessful
authority initialization still published new dimensions, undoing a resize made
earlier edit history unusable, and redo generated new stable IDs for expanded
tiles. The resize guard also allowed closed/faulted editors. These paths are now
repaired in the uncommitted working tree based on
`052e1acb02b790641d63466de40c91d56028383b`.

## Implementation and scope

The settings Set action calls `Editor.ResizeMap` before any map mutation. Invalid
dimensions or a failed operation retain the user's dimensions and show the error
in the settings panel. Inputs use the existing model's 4,096 dimension limit;
the total 16,777,216-cell bound is checked before candidate allocation. These
are validity limits, not a claim that a maximum-size map fits every computer.

The owned `editing.Resize` planner takes authoritative snapshot state and the
loaded world's default turf/area paths. It preserves retained IDs, opaque content
and sparse empty cells, and generates added-cell IDs once. It clones retained
states rather than first copying all removed display tiles. Validation and
source isolation are covered by focused tests.

Each changed size is a separate local maintenance document with a distinct
document ID. Its local executor and operation history remain available for undo
and redo, so later undo of an earlier edit still reaches its original engine.
Resize undo reinstalls a retained local history context; it is not a new v1
network operation. Normal edits still use the shared operation engine and
actor-scoped inverse operations. Network resize remains disabled under the
approved maintenance deferral; no public protocol/schema changed.

Both the current and target map hashes must match the captured resize boundary
before switching contexts. A deliberately changed inactive executor initially
bypassed this check in the first candidate implementation; the added regression
reproduced it, and the final implementation rejects it without moving history.
Failed undo is retryable. History-generation keys can resume a prior local
context; the separate asynchronous attachment generation always increases.
External attachment changes fence both old edit and resize commands.

The target display is built and validated before authority, dimensions, active
level or command ownership changes. Successful installation clamps removed Z
levels, resets the active map's selected tool, and refreshes the canvas. This
does not add arbitrary recovery of uncaptured edits or incomplete capture faults.

## Evidence

Raw evidence is in `.artifacts/resize-audit-2026-09-06/`.

| Gate | Observed result |
| --- | --- |
| Before repair | `authority-red.txt` reproduces resize enabled after close/capture error. `workspace-red.txt` reproduces the three dimension/history/ID failures. |
| Checkpoint preconditions | `precondition-red.txt` reproduces installation of an unexpectedly changed inactive executor before the boundary-hash check. |
| Focused packages | Owned editing, editor and settings passed. Tests cover shrink/grow planning, sparse and unknown content, isolation, invalid bounds/defaults, settings delegation and retained failure input. |
| Real hidden workspace | Tests cover exact resize/earlier-edit undo, expansion redo IDs, a five-action edit/resize chain in both directions, retry after failed undo, changed checkpoint rejection, new document identities, selection/Z cleanup and later attachment fences. |
| Maintained repository gate | `task verify` passed lint (zero issues), contracts, all Go tests, pinned Rust test/fmt/clippy, parser release build and Windows desktop build. The Rust crate has zero unit tests. |
| Affected race suites | Owned editing 1.093 s; editor 1.650 s; settings 1.152 s; command storage 1.072 s. |
| Hidden workspace under race | Explicit resize, selection and Save suites passed in 6.175 s with `APHELIONDMM_GL_TEST=1`. |
| Produced smoke command | Parser open/save/reparse and authenticated two-client loopback collaboration passed, with accepted revisions 1/2, identical client hashes, leave and natural shutdown. |

The inherited ImGui compiler warning remains. An early workspace compile failed
because the tool interface has no `Reset` method; the adapter now uses its existing
`OnDeselect` hook. The final source passed all gates above. The settings test calls
its actual Set action with a controlled editor; the workspace tests invoke the
real editor under OpenGL. These do not substitute for human button/input testing.

Toolchains: Go 1.25.13 Windows/amd64, Rust 1.82.0 GNU, Task 3.53.1 and
golangci-lint 2.12.2. SHA-256 provenance:

- Desktop: `0ead3460a476bcfe683cea7e7eb916be428aa5bb3492464af2192b8e858d24f2`.
- Smoke command: `9c1ee147eda10c438e7976ded05879c875819e3f3a48638919e03650573736d2`.
- Smoke input/output: `7d044f7165ca68f32d823f463c6810876c91aa13934d4880fd96810469e9c9be`.
- Both smoke clients: `198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa`.

The smoke reports revision `undefined`; `manifest.json` records working-tree and
binary hashes. Documentation and that manifest were completed after code gates.

## Performance findings and remaining work

Same-size requests return before snapshot reading or mutation. A fresh
`testing.AllocsPerRun(20, ...)` regression observes zero allocations at 100,
1,000 and 10,000 cells, while checking unchanged authority version, display tile
identity and empty history. This is an allocation assertion on the no-op path,
not a paired latency benchmark, changed-size speedup or desktop-frame result.

Changed-size operations still perform full snapshot/hash validation and retain
local executor checkpoints for correct undo. Profile retained memory over repeated
resizes and redo branching, including command storage's truncated backing arrays.
The source also shows `pushAcceptedCommand` inserting into the currently active
command stack; a delayed acknowledgement after a tab switch needs a fresh
reproduction and per-document ownership audit. Both are follow-up investigation
targets, not repaired or measured findings in this report.

The [editor workplan](../superpowers/plans/2026-09-06-editor-qol-workplan.md) and
[repo-wide performance plan](../superpowers/plans/2026-09-06-admm-performance-followup.md)
retain placement preview, configurable bindings/grid steps, navigation/stamps,
partial-capture recovery, representative desktop/GPU timing, engine/projection
copies, durable history, concurrent load/recovery, parser/icon lifetime and long
lifecycle measurements. Human acceptance, PostgreSQL, hosted and Meridian gates
remain separate and unrun here. No commits, pushes or protected infrastructure
changes were made.

Reproduce with pinned tools from the repository root, checking each exit code:

```powershell
go test ./internal/aphelion/editing ./internal/app/ui/cpwsarea/wsmap/pmap/editor ./internal/app/ui/cpwsarea/wsmap/pmap/psettings -count=1 -timeout 90s
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/app/ui/cpwsarea/wsmap -run '^Test(Resize|Selection|SaveAcknowledgementBoundaries)' -count=1 -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
New-Item -ItemType Directory -Force .artifacts/resize-audit-2026-09-06 | Out-Null
go build -o .artifacts/resize-audit-2026-09-06/smoke.exe ./cmd/apheliondmm-smoke
& ./.artifacts/resize-audit-2026-09-06/smoke.exe
```
