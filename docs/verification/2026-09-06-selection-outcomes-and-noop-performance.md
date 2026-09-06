# Selection outcomes and unchanged-gesture performance

Fresh source inspection and failing regressions found two defects in the current
working tree. Rectangular rotation and nudge changed the Grab bounds immediately,
but rejection and undo changed map state without restoring selection geometry.
Separately, committing one captured but unchanged tile copied the entire executor
snapshot before detecting that there was no operation to submit.

Both paths are repaired. Changes remain uncommitted, based on
`052e1acb02b790641d63466de40c91d56028383b`; prior unrelated work is preserved.

## Selection behavior and ownership

`internal/aphelion/editing/selection_history.go` holds geometry only. A selection
owns a sequence of transforms, with each transform linked to its own operation
outcome and history callbacks. Rejection or undo disables that transform; redo
reenables it. A newer active transform keeps its own shape. If consecutive
transforms fail, bounds resolve through them to the last active shape or the
original selection. No map content is copied into this metadata or restored from
it, and nothing is added to the collaboration protocol.

Narrow editor/tool adapters register the observer while the transform commits,
copy it into the operation completion and undo/redo closures, and use the existing
attachment-generation fence. Reset, deselection and a new explicit selection
replace the selection's geometry context. A different map cannot receive an old
selection update. A newer open mouse gesture retains its preview until release.
Immediate invalid or cancelled actions discard their provisional geometry entry.
Unchanged acknowledgement geometry does not trigger another selection-content copy.

Reproductions and coverage:

- Before repair, delayed rectangular rotation and nudge rejection retained the
  transformed bounds, and undo after nudge retained the nudged bounds. Retained
  failing logs are `geometry-red.txt`.
- Hidden workspace tests use the real `PaneMap`, `Editor`, `ToolGrab`,
  `NetworkExecutor` and operation engine with a controlled in-memory transport.
  They check delayed and immediate rejection, local rotation/nudge undo and redo,
  consecutive rejections in both callback orders, new selections, deselection and
  inactive-tab outcomes. The controlled reverse order tests callback robustness;
  it does not claim that normal WebSocket delivery reorders server messages.
- A tool-entry test performs nudge, mouse start/move/release, late outcomes and
  reset. An older rejection cannot replace the newer open preview; both failed
  transforms restore the original bounds after release. Existing workspace tests
  separately cover map hashes, stable IDs, remote passed-over edits and old
  attachment completions.
- These cases do not replace real socket-backed mouse interaction, wider
  temporary-tool/multiple-tab sequences, large or hidden-type transforms, or
  human acceptance. Those remain in the editor workplan.

## No-op snapshot repair and measurement

`commitOperation` now compares the captured before/after tile states first. An
empty change set clears the completed gesture without requesting authority,
creating an operation or entering undo history. Real edits still read the
executor snapshot before submission. A failed read retains the original capture
and blocks Save; a focused regression successfully retries that same edit.

The failing `no-op` regression observed one full snapshot call for an unchanged
gesture. The repaired path observes zero. The original failing output is retained
in `noop-red.txt`; the retry guard is covered by
`TestChangedGestureRetainsCaptureWhenSnapshotFails`.

`BenchmarkUnchangedGestureSnapshotOrdering` exercises the real capture and commit
methods for one unchanged tile on 100, 1,000 and 10,000 synthetic cells. The
`read_first_control` explicitly calls the removed executor snapshot read before
the same unchanged commit; `defer_until_changed` uses the repaired path. This is
a controlled attribution comparison in one test binary, not a historical checkout
comparison. Both variants validate an identical final map hash, revision zero,
no pending work and no undo entry. Construction and final validation are untimed.
Stable IDs and the document ID are fixed. All 30 final process/size combinations
were checked for identical per-size hashes across variants and trials.

Five independent process trials per variant used 500 ms benchmark calibration.
Control/candidate order alternated by trial. No build or other test ran during
timing. Values below are medians across the five per-trial means:

| Map cells | Control time | Repaired time | Control B/op | Repaired B/op | Control allocs/op | Repaired allocs/op |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 100 | 49.533 us | 1.636 us | 62,576 | 1,712 | 516 | 15 |
| 1,000 | 507.808 us | 1.675 us | 610,872 | 1,712 | 5,016 | 15 |
| 10,000 | 4,140.245 us | 1.359 us | 6,085,044 | 1,712 | 50,016 | 15 |

Timing variation is retained: repaired trial means span 1.621–1.668 us at 100
cells, 1.660–1.690 us at 1,000, and 1.354–1.398 us at 10,000. The 10,000-cell
control spans 4.094–4.171 ms. This removes map-sized copy work from this specific
no-op commit. It does not measure rendering, cancellation restoration, a large
selection's touched-tile comparisons, input-to-visible latency or total frames.

## Verification and artifacts

Raw evidence is under `.artifacts/selection-outcomes-2026-09-06/`.

| Gate | Result |
| --- | --- |
| Focused owned editing, tools and editor tests | Passed, including invalid/cancelled actions, geometry outcomes and capture retention on snapshot failure. |
| Affected race suites | Owned editing, tools and editor packages passed in 1.063 s, 1.145 s and 1.278 s. |
| Hidden workspace | Explicit selection and Save tests passed; the final run under race passed in 5.449 s with `APHELIONDMM_GL_TEST=1`. |
| Maintained repository gate | `task verify` passed lint/contracts, all Go tests, pinned Rust test/fmt/clippy, parser release build and Windows desktop build after the final production change. The Rust crate reports zero unit tests. |
| Final benchmark fixture | Only the benchmark's fixed-ID setup/logging changed afterward. It was recompiled, executed in five alternating trials and checked for matching hashes; the final maintained `task lint` passed with zero issues. |
| Produced parser/loopback smoke command | Passed open/save/reparse, two authenticated clients, accepted revisions 1 and 2, matching client hashes, leave and natural shutdown. Input/output file SHA-256 matched. |

The initial repository gate requested one tagged-switch cleanup in the new test;
it was applied and verification rerun. That log is retained. The inherited ImGui
`memset` compiler warning remains. The smoke command is not a human desktop
interaction test, and default skips are not fresh PostgreSQL or external evidence.

Toolchains: Go 1.25.13 Windows/amd64, Rust 1.82.0 GNU, Task 3.53.1 and
golangci-lint 2.12.2. SHA-256 provenance:

- Desktop: `c5ed39d0bc6162dbf0d1976119233b2682097482dc8aa689ef482a67e2c5cf90`.
- Smoke command: `9dd8f783724bba8c3de7846b266cb25b4adea3b78f2bfbe8a6e1b2de134bec89`.
- Final benchmark binary: `fa0d12c1e81b6cb3899e76d5bb84b89668e6fc44168399d48507dff21acae734`.
- Smoke input/output: `7d044f7165ca68f32d823f463c6810876c91aa13934d4880fd96810469e9c9be`.
- Both smoke clients: `198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa`.

Smoke reports revision `undefined`; binary/source hashes in `manifest.json`
provide provenance. Final synthetic map hashes are in `fixture-hashes.json`.

The benchmark launcher first rejected an unquoted dotted test flag in PowerShell.
It was rerun with complete quoted arguments; `bench-launch-failed.txt` preserves
that error. Thirty valid measurement rows are in `bench-results.json`; medians
and ranges are in `bench-summary.json`. Individual process outputs are named
`bench-<trial>-<variant>.txt`.
The earlier exploratory run used generated stable IDs. Its output and binary are
retained under `exploratory-random-ids/`; the table above uses only the fixed-ID
repetition. There is no claim that those preliminary hashes matched across runs.

## Reproduction and remaining work

Use the pinned Go 1.25.13 and Rust 1.82.0 GNU toolchains from the repository root:

```powershell
go test ./internal/aphelion/editing ./internal/app/ui/cpwsarea/wsmap/tools ./internal/app/ui/cpwsarea/wsmap/pmap/editor -count=1 -timeout 90s
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/app/ui/cpwsarea/wsmap -run '^Test(Selection|SaveAcknowledgementBoundaries)' -count=1 -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
task verify
$selectionEvidence = Join-Path $PWD '.artifacts/selection-outcomes-2026-09-06'
New-Item -ItemType Directory -Force $selectionEvidence | Out-Null
go test -c -o (Join-Path $selectionEvidence 'editor-bench.test.exe') ./internal/app/ui/cpwsarea/wsmap/pmap/editor
& (Join-Path $selectionEvidence 'editor-bench.test.exe') '-test.run=^$' '-test.bench=^BenchmarkUnchangedGestureSnapshotOrdering' '-test.benchmem' '-test.benchtime=500ms' '-test.timeout=60s'
```

Check native exit codes. For paired trials, filter one variant per process and
alternate the order as described above; the last command alone runs both variants
in fixed order and is only a reproduction smoke.

Continue the [editor workplan](../superpowers/plans/2026-09-06-editor-qol-workplan.md)
and the [repo-wide performance workplan](../superpowers/plans/2026-09-06-admm-performance-followup.md).
Remaining work includes placement preview, configurable bindings,
navigation/stamps, broader selection ownership transitions, representative desktop
timing, engine/projection copies, durable history, concurrent load campaigns,
parser/FFI, rendering/cache and long lifecycle measurements. PostgreSQL, hosted,
external integration and human tool acceptance remain separate gates.

The subsequent [mirror/authority pass](2026-09-06-selection-mirrors-and-authority.md)
adds H/V reflection and closes two later reproduced gaps: an unchanged transform
could mask an earlier selection undo, and `Editor.CommitOperation` could enter
`commitChangesLegacy` after a capture fault or missing executor. The results above
describe the earlier source/measurement checkpoint; consult the follow-up for
those repairs and current verification. Resize maintenance remains separate.
