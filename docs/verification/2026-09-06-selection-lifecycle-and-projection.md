# Selection lifecycle and indexed projection qualification

Baseline commit: `052e1acb02b790641d63466de40c91d56028383b`, with the earlier
uncommitted rotation, shortcuts and streaming-hash changes already present.
This pass inspected current source and reproduced failures before fixing them.
It continues the [editor workplan](../superpowers/plans/2026-09-06-editor-qol-workplan.md)
and [repo-wide performance workplan](../superpowers/plans/2026-09-06-admm-performance-followup.md).
Changes remain uncommitted; no protected infrastructure entry point changed.

The later [nudge/network follow-up](2026-09-06-selection-nudges-and-network-transitions.md)
releases the retained editor journal described below, adds one-tile nudges and
qualifies specific network transitions. This report records the preceding pass.

## Selection lifecycle findings and repairs

The inherited Grab motion path could replace moved instance IDs, replace IDs on
passed-over tiles, and restore a previous gesture's stale background over a newer
edit. Deselecting during a drag left the speculative mutation behind. Further
regressions showed that Escape did not cancel through the tool frame handler,
resize remained allowed during a drag, attachment close left a live drag handle,
and movement could continue after switching to another valid Z level.

`internal/aphelion/editing/move.go` now owns source/background state for one
cancellable gesture. Narrow editor and Grab adapters capture each tile before
mutation and submit one operation on release. Moved collaboration IDs and hidden
instances survive; passed-over tiles regain their original contents and IDs.
Each new gesture reads current map contents. Visibility is frozen at gesture
start. Whole-selection bounds checks reject an invalid preview before mutation.

Cancellation restores the original map without adding history. Escape reaches
the tool handler while the canvas owns the active mouse item. Level-switch
cancellation refreshes the original level. Resize is refused while an edit is
unfinished. Attachment reset closes the old handle without restoring stale
contents over the replacement snapshot. The previous motion implementation is
retained in an ownership removal comment for upstream provenance.

The Move background cache retains only the source and current destination after
each preview. **The editor's before-change journal still retains first-touched
coordinates until release**, so total gesture memory is not bounded solely by
selection size. Legacy paste background bookkeeping also remains a cleanup lead.

## Projection findings and repair

`Projection.Visible` previously cloned acknowledged state, then cloned, indexed,
validated and hashed the entire map for each pending operation. The new
`internal/aphelion/collab/client/projection_index.go` creates one owned snapshot
and reuses per-call coordinate and stable-ID ownership indexes.

Every pending batch is checked in full before mutation: bounds, duplicate
coordinates, current before-state, valid IDs, duplicate after-state IDs and
collisions with unchanged tiles. IDs may relocate between affected tiles in
either change order. Invalid/conflicting operations remain pending but do not
partially change the visible map. Requests, headers and preconditions remain
unchanged, and callers cannot mutate the acknowledged snapshot through results.

Baseline validation happens once. Publicly constructed malformed baselines use
the old application path as a compatibility fallback, including repairable
invalid state. Accepted-operation digest verification is unchanged. Submit and
Accept still have separate clone/hash work requiring further attribution.

The fixed-seed differential suite compares 40 sequences of 16 prefixes against
the unchanged application routine: **640 comparisons**, including stale
preconditions, invalid batches, deletions and ID swaps. Separate cases cover
sparse appends, collisions, alias isolation and malformed baselines. Before the
repair, eight one-tile edits added 24,368 allocations above a 3,001-allocation
empty-pending projection; the regression now allows at most 300 additional
allocations and passes.

## Matched component measurements

Raw evidence is in ignored `.artifacts/selection-projection-2026-09-06/`, including
binary/source hashes in `manifest.json`, individual trials, `measurements.csv`
and `summary.json`. Control already includes the previous streaming-hash change;
these percentages measure indexed projection separately from that earlier work.

Both executables use the same checked-in performance fixture, Go 1.25.13,
Windows amd64, Intel Core i7-9750H and one benchmark CPU. One complete paired run
was excluded as warmup. Five measured pairs alternated execution order with a
500 ms target, without concurrent builds or tests:

```powershell
& .artifacts/selection-projection-2026-09-06/control.exe '-test.run=^$' '-test.bench=BenchmarkAuditProjectionVisible$' '-test.benchtime=500ms' '-test.count=1' '-test.benchmem' '-test.cpu=1'
& .artifacts/selection-projection-2026-09-06/candidate.exe '-test.run=^$' '-test.bench=BenchmarkAuditProjectionVisible$' '-test.benchtime=500ms' '-test.count=1' '-test.benchmem' '-test.cpu=1'
```

The fixture has 1,000 cells, one prefab and one direction variable per cell.
Times below are medians and ranges of five benchmark trial means, not request
latency percentiles. Allocation columns are medians.

| Pending edits | Control ms/op (range) | Candidate ms/op (range) | Control B/op | Candidate B/op | Control allocs/op | Candidate allocs/op |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 0 | 0.330 (0.326–0.368) | 0.331 (0.316–0.416) | 433,152 | 433,152 | 3,001 | 3,001 |
| 1 | 1.469 (1.440–1.616) | 0.812 (0.767–1.061) | 1,194,032 | 897,040 | 6,047 | 3,057 |
| 8 | 9.763 (9.625–11.152) | 0.797 (0.761–0.962) | 6,520,192 | 899,728 | 27,369 | 3,078 |

Eight-pending projection used **86.2% fewer bytes**, **88.8% fewer allocations**
and **91.8% less component time** by these medians. The zero-pending path has
unchanged allocation and overlapping timing. One-pending and eight-pending
candidate times overlap; do not infer that more edits improve speed. These are
synthetic component results, not representative-map or desktop-frame results.

Both retained executables subsequently passed `TestPerformanceWorkloads`, which
checks fixture operations, projection and map round-trip behavior. Their fixture
hashes match at 100, 1,000 and 10,000 cells:

| Cells | Canonical hash |
| ---: | --- |
| 100 | `b75c95a4f89e4c8d823eac2b0bdcdf37e1c1042b0c80a589695aef608b9effc0` |
| 1,000 | `31f3bd017a56e4c81f19d1d47b57e610a7de74d41c20377b41a6f63691601862` |
| 10,000 | `ac8ddd4b8e3ae97e33609bf6ec9649fd673d030051975d24b56ff7541fe9e47c` |

## Qualification and open gates

| Evidence | Result |
| --- | --- |
| Focused behavior | Editing, client, Grab and editor tests passed after the reproduced failures. An initial invalid-level test fixture was corrected to use two real levels before counting its failing assertion as evidence. |
| Hidden real workspace | Explicit `APHELIONDMM_GL_TEST=1` execution of `TestSelectionMoveWorkspaceLifecycle` and `TestSaveAcknowledgementBoundaries` passed. Covers no durable mutation during preview, cancellation/no history, moved/passed IDs, exact undo hash, redo identity, valid level-switch cancellation, existing rotation/tool shortcuts and Save boundaries. |
| Repository gate | Final `task verify` passed: lint/contracts, all Go tests, Rust fmt/Clippy/test, parser release build and desktop build. Rust reported **0 unit tests**. The inherited ImGui C++ `memset` warning remains. |
| Race | Final `go test -race ./internal/aphelion/... -count=1 -timeout 120s` passed. |
| Maintained smoke executable | Rebuilt executable passed fixture open/save/reparse, loopback service launch, two-client operations/convergence, leave and natural shutdown. Saved bytes match the fixture; accepted revisions are 1 and 2, and both clients reached `198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa`. |
| Build provenance | Desktop SHA-256 is `509cdc692817708efa17c7b80ad9411fca688fee7a33d134165ab6d52aee20c6`; benchmark executable and source hashes are in the retained manifest. Smoke report revision is `undefined`, so it is not used as commit provenance. |
| Unrun | End-to-end network rejection/reattachment during a live Grab gesture; broader tab/temporary-tool combinations; representative desktop/GPU input-to-visible profiling; real PostgreSQL; hosted container/CI; real external integrations; production load and human interaction acceptance. |

Normal Windows account access was needed for the maintained lint and real
workspace gates after sandbox account/path-resolution failures. Those initial
environment failures are not counted as passes.

Next, extend gesture coverage to network transitions and measure the remaining
before-change journal cost. Continue the repository-wide load, engine-copy,
durable-store, parser/FFI, rendering/cache and lifecycle work. Mirrors, nudges,
paste preview and shortcut rebinding remain planned; this pass does not close
the overall audit or feature goal.
