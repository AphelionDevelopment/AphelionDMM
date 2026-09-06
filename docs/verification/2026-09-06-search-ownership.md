# Search ownership and capture qualification

Historical qualification for the ownership/capture pass. The subsequent
[replacement-validation report and stopping point](2026-09-06-search-after-values-and-stopping-point.md)
records the latest repair, build identity and remaining work.

Baseline commit: `052e1acb02b790641d63466de40c91d56028383b`, including the current
uncommitted repairs and the preceding search performance pass. Current source,
not the earlier workplan's completion statements, drove this investigation.
Artifacts are in `.artifacts/search-ownership-2026-09-06/`.

## Reproduced failures

- Search retained display-instance pointers after same-editor snapshot
  application replaced those instances. Actual F3 navigation highlighted detached
  instances after shrink, resize undo/redo and explicit snapshot refresh.
- Select, Delete, Replace, Delete All, Replace All, next and previous result
  actions panicked when the current editor had closed. A cached row could also
  select an old map's instance through a newly active editor.
- Search navigation operated on a floating selection preview. A query during
  that preview retained objects that cancellation subsequently replaced.
- Delete/Replace actions changed the display even with an existing capture
  fault. A new failure while capturing a later result could also leave an earlier
  bulk result modified and retain unused before-states. The operation was refused,
  but its display side effects had already happened.

Corrected regression evidence is `row-red.txt`, `red-corrected.txt`,
`gesture-red.txt`, `operation-red-valid-history.txt` and `capture-red.txt`.
Earlier harness attempts exposed a value/pointer mismatch, an unsafe GetTile
assertion on a removed coordinate and a missing fixture history stack. Those
attempts are retained but are not passing evidence. The corrected health-guard
control uses an overlay removing only the guard; healthy operations pass there
while all four faulted-mutation cases fail.

## Repair boundaries

`Editor.MapViewVersion` exposes a UI-thread display generation and readiness.
Captures, authority installation and attachment changes advance that generation;
it is independent of durable document revision. Close marks the view unavailable.
An unfinished gesture makes the view temporarily unavailable for search actions
and query rebuilds, so moving a preview does not force repeated full-map searches.

Search records its originating editor and display generation. It checks them
before rendering or navigating and coalesces invalidations into one query when
the view becomes ready. Automatic refresh within the same map preserves XY
filter bounds and resets navigation. Explicit queries and map switches retain
their previous filter-reset behavior. F3 chooses its next index after refresh.
A stale row or bulk-action index refreshes the list and returns without applying
the old action to a different result. Closed-map actions release cached results.

`Editor.CanStartMapEdit` prevents independent search mutations during another
gesture, after close/history disposal, or with a capture/authority fault.
Read-only navigation remains separate from mutation eligibility. Submitted
network operations can still coexist with normal speculative editing; the guard
does not treat waiting for an acknowledgement as a durable failure.

`Editor.CommitInstanceBatch` validates current instance membership and captures
every target before any display deletion/replacement. Failed preflight releases
the before-states it acquired, leaves display contents intact and reports the
failure. Invalid content retains the existing fault/Save guard. A detached target
does not poison an otherwise healthy map. Successful batches use the existing
local/network executor and actor-scoped history path. Returning from submission
does not imply a network acknowledgement.

The adapters are marked Aphelion-owned spans in the editor/search boundary. No
protocol, schema, dependency, generated asset or protected infrastructure changed.
These changes do not establish complete safety/performance for arbitrary-size
bulk actions; remaining validation and wire-size cases are listed below.

## Fresh qualification

| Evidence level | Result |
| --- | --- |
| Search component | All 14 top-level tests pass under race in 1.291 s. This includes the original query/table regressions, closed/switched maps, stale-row refusal, automatic filter preservation, unchanged-view reuse, local action history and capture failure. |
| Local mutation authority | Row Delete/Replace and Delete All/Replace All each publish one expected revision on a valid local document, then restore exact map hashes through undo and redo. Rendering is adapted in these tests; scheduled bucket jobs are not executed. |
| Actual workspace | All 47 top-level hidden GLFW/OpenGL tests pass under race in 12.063 s, with no skips. New coverage includes F3 after shrink/undo/redo/refresh, complete local/remote result traversal, preview cancellation, batch network acceptance/rejection/undo and detached-target preflight. |
| Other affected packages | Owned search 1.284 s; app 1.980 s; pane 1.191 s; canvas 2.180 s; editor 2.027 s; settings 1.161 s; tools 1.177 s, under race. Packages without tests remain labelled in the raw log. |
| Repository/build | `task verify` passes zero-issue lint, contracts, all Go tests, Rust test/fmt/clippy, parser release and actual desktop build. Rust currently has zero unit tests. Default GL skips are covered separately above. |
| Produced command | Fresh parser/loopback smoke passes open/save/reparse, two authenticated clients, revisions 1/2, identical client hashes, leave and shutdown. This command does not exercise the desktop UI. |

The network workspace test verifies one two-tile submission, refusal to Save or
insert history before acknowledgement, exact rejection rollback with retained
conflict state, and exact accepted-operation undo. These are controlled in-process
transport/engine fixtures, separate from the produced loopback smoke and hosted
network/load acceptance.

Go 1.25.13 windows/amd64 and Rust 1.82.0 GNU remain pinned. Recorded tools are
Task 3.53.1, golangci-lint 2.12.2 and GCC 15.2.0. The inherited ImGui C++ memset
warning remains. All 527 source/dependency files hashed before the final gates
remain unchanged afterward. Documentation was finalized after qualification.

## Performance preservation

An unchanged 10,000-cell, 16-variant view performs 100 repeated freshness checks
with zero allocations and retains its result storage. The check compares editor
identity, generation and readiness; it does not traverse the map.

Five alternating control/candidate pairs rerun the same five workloads and
50-iteration instrumentation from the preceding search audit: 50 samples, 2,500
measured calls. The control already includes that pass's single-scan queries and
clipped table. No build or test ran concurrently with these trials.

| Workload | Control -> candidate median ms | Median Go B/op | Median Go allocs/op |
| --- | --- | --- | --- |
| Path query, 1 variant | 0.401222 -> 0.378914 | 311,287 -> 311,379 | 25 -> 25 |
| Path query, 16 variants | 0.632442 -> 0.637652 | 364,664 -> 364,680 | 188 -> 188 |
| Path query, 128 variants | 0.683430 -> 0.574916 | 370,921 -> 370,855 | 1,036 -> 1,036 |
| Table, 100 results | 0.206556 -> 0.135304 | 20,184 -> 20,184 | 575 -> 575 |
| Table, 5,000 results | 0.218738 -> 0.139050 | 20,312 -> 20,312 | 591 -> 591 |

All 14 ordered query/draw/scroll records match across all ten processes. Table
allocation counts match exactly; query allocation counts vary slightly across
samples with unchanged medians and logging still enabled. Timing ranges overlap
for every workload, so this correctness pass makes no speedup claim. For example,
5,000-row control spans 0.134-0.255 ms and candidate 0.125-0.177 ms. Raw samples,
summary, equivalence records and the executable trial script are retained.

The previous audit's fixture boundaries still apply: synthetic matching queries,
headless ImGui draw-data generation, bottom viewport after calibration, and no
GPU rasterization. Go allocation excludes native/GPU/process memory. Mutation
preflight and large bulk edits were not timed by these query/table benchmarks.
Do not compare this table directly to the unoptimized search baseline and call
the difference a gain from ownership checks.

SHA-256 identities:

- Control: `ce781109eb1e57ed9cee9b07790b8b3517b58e70a4938613aeb53fc7e5f78baf`.
- Candidate: `813581e53cfd06d9c4c18db2d92b9e96a6f073a197e0360c2d88c9ac0343c3a1`.
- Desktop: `11ba3bc2bcb8701bc1f129f965e622faa34fceac729c4492c443c77871c84b9d`.

The smoke revision field remains undefined; binary/source hashes identify the
run. The manifest uses relative paths. Work remains uncommitted.

## Reproduction and remaining work

From the repository root with pinned toolchains; check each native exit code:

```powershell
go test ./internal/app/ui/cpsearch -run '^TestSearch' -count=1 -v
$env:APHELIONDMM_GL_TEST = '1'
go test ./internal/app/ui/cpwsarea/wsmap -run '^TestSearch' -count=1 -v -timeout 60s
go test -race ./internal/aphelion/search ./internal/app/ui/cpsearch ./internal/app ./internal/app/ui/cpwsarea/wsmap ./internal/app/ui/cpwsarea/wsmap/pmap/... ./internal/app/ui/cpwsarea/wsmap/tools -count=1 -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
& './.artifacts/search-ownership-2026-09-06/run-trials.ps1'
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
```

The trial script requires the preserved binaries. Restart with the rebuilt
`dst/StrongDMM.exe` for human acceptance. Next qualification must cover oversized
bulk actions and wire-size limits, invalid replacement after-values, failures
after successful capture, hidden/unknown types and real large maps. Preserve
one-logical-action history and explicit batching semantics. Profile preflight,
capture, query refresh and render stages separately, including many instances
per tile and rapid accepted edits with Search visible.

Actual row-button clicks during filter/list refresh, scaled fonts, keyboard
layouts, mouse focus and human usability remain unrun. The callback, shortcut,
model and hidden-workspace evidence above does not replace them. Z-level
filtering, user-rebindable shortcuts and the wider editor QoL backlog remain
in the [QoL workplan](../superpowers/plans/2026-09-06-editor-qol-workplan.md).
Continue the [repo-wide performance workplan](../superpowers/plans/2026-09-06-admm-performance-followup.md);
PostgreSQL, hosted CI, external integrations and production-scale acceptance
remain separate unrun gates. The overall goal remains active.
