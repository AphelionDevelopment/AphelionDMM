# Selection tools, shortcuts, and canonical-hash follow-up

Baseline: `052e1acb02b790641d63466de40c91d56028383b`. The working tree was clean
at the start. This pass inspected current implementations and reproduced defects;
earlier audit reports were used as leads, not accepted as current verification.
All changes remain uncommitted. The
[workplan](../superpowers/plans/2026-09-06-editor-qol-workplan.md) records remaining
selection work and the complete repo-wide performance scope.

## Changes and evidence

| Finding / addition | Result and verification |
| --- | --- |
| Grab had no rotation action | Added 90-degree left/right toolbar actions and `[` / `]`. The bottom-left anchor stays fixed; width and height exchange. Rotation uses current contents and one explicit operation, preserving stable instance IDs and hidden contents. |
| Undo and asynchronous rotation | Focused local and delayed-acknowledgement tests verify directions, IDs, undo/redo, and no undo entry before acknowledgement. Rectangle tests verify coordinate mapping, source/destination union, opaque variables, inverse/four-turn behavior and inherited directions. |
| Unsafe orientation / map boundary | Whole-action rejection for out-of-bounds destinations or unsupported direction/offset expressions. Recognized integer/named `dir` and numeric pixel/step offsets rotate. Unknown types and unrelated variables survive. No custom pipe/cable/type/transform rules are guessed. |
| Ctrl+S discarded Grab selection | Reproduced through actual ImGui input: the temporary-tool handler selected Pick while Ctrl+S was pressed. It now ignores command modifiers and text editing. Text-field regression also covers typing R. |
| Drag could resurrect a stale rotated copy | Reproduced an acknowledged direction change after selection, followed by a new drag. Grab now refreshes source contents at gesture start. This does not close every inherited background-restoration or move-identity issue; those remain explicit workplan items. |
| Rotation could edit a non-visible level | Reproduced and rejected at the editor boundary; toolbar/shortcut availability also requires the selected level to be visible. |
| Missing shortcut discovery | Added Help > Keyboard Shortcuts / F1 with filtering. Registered bindings and keypad alternatives are read from the live registry and deduplicated across panes. Held/mouse controls are explained separately. |
| Additional useful bindings | Added permanent Pick/Delete/Replace on 5/6/7 and keypad equivalents, Esc deselection, and retained Shift+= zoom-in. Exact modifier matching prevents unrelated bare-key fallthrough. Idle matching checks the action key before polling modifiers; its regression verifies zero unnecessary modifier reads. |
| Canonical encoding retained the entire map byte stream | Replaced the growing buffer with invocation-local 4 KiB scratch feeding the same SHA-256 digest. Validation, sorting, domain tag, field order, integer encoding and length prefixes are unchanged. Golden and independent buffered-oracle tests pass, including long values, buffer boundaries, empty values, Unicode and embedded zero bytes. |

Rotation deliberately replaces visible destination contents, as movement does;
undo restores overwritten contents. It does not clip a rectangle to fit. Numeric
orientation values may be normalized and redundant inherited overrides removed.
It does not provide arbitrary-angle rotation, in-drag rotation, configurable
pivots, custom environment-specific type rotations, or shortcut rebinding.

## Qualification

Raw files are under ignored `.artifacts/performance-2026-09-06/`.

| Evidence level | Result |
| --- | --- |
| Focused regressions | Passed. New behavior and reproduced shortcut, stale-selection, visible-level and idle-polling failures were tested before their implementation/fix. |
| Repository gate | `task verify` passed on the final behavior: lint, contract/buildcheck/manifest gates, all Go tests, pinned Rust test/fmt/Clippy, parser release build and Windows desktop build. Rust reported **0 unit tests**; this is not Rust behavior coverage. |
| Race | `go test -race ./internal/aphelion/... -count=1 -timeout 120s` passed. The subsequent shortcut polling-order change received a fresh focused race check and another full repository gate. |
| Real hidden workspace | Explicit `APHELIONDMM_GL_TEST=1` gate passed through real PaneMap construction, registered left/right rotation and 5/6/7 shortcuts, then acknowledged/failed Save checks. The default full-suite skip is not counted as this evidence. |
| Maintained smoke executable | Passed real fixture open/save/reparse and loopback two-client operations, convergence, leave and natural shutdown. Accepted revisions were 1 and 2; both clients reached hash `198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa`. |
| Toolchain doctor | Passed: Go 1.25.13, Rust 1.82.0, Task 3.53.1, golangci-lint 2.12.2, GCC 15.2.0. |
| External / human | PostgreSQL, local hosted-container lifecycle, hosted CI, real Meridian integrations, production load, representative-map GPU/frame profiling, and human UI acceptance were not rerun in this pass. Earlier evidence remains dated separately. |

The Windows sandbox prevented account/path resolution for real PaneMap
construction and lint. Those gates passed when rerun with normal account access;
the initial environment failures were not counted as successful checks. The
inherited ImGui C++ `memset` compiler warning remains.

## Matched component measurements

Control and candidate executable/source hashes are retained in `manifest.json`.
Both use the same checked-in `internal/aphelion/perfaudit/performance_test.go`,
Go 1.25.13, Windows amd64, an Intel Core i7-9750H, and `-test.cpu=1`. No validation,
operation, hash, or durability work is disabled. Fixture correctness gates and
two-client convergence are separate from timing. The component fixture contains
one prefab and one direction variable per cell; it is not a representative game
map or a GUI/frame benchmark.

One complete paired run was excluded as warmup, then five pairs were measured
with alternating control/candidate order and a 300 ms benchmark target. No
builds or tests ran concurrently with those measurements. Use
`qualified-{control,candidate}-{2..6}.txt` and `summary.json`; earlier unqualified
files include an interrupted run overlapping compilation and are excluded.

Medians of the five benchmark trial means:

| Workload | Control ms/op | Candidate ms/op | Control B/op | Candidate B/op |
| --- | ---: | ---: | ---: | ---: |
| Hash, 100 cells | 0.095 | 0.072 | 49,576 | 21,896 |
| Hash, 1,000 cells | 1.015 | 0.835 | 502,400 | 245,344 |
| Hash, 10,000 cells | 13.754 | 8.770 | 6,207,248 | 2,018,032 |
| Clone/apply, 100 cells, no history | 0.197 | 0.160 | 145,160 | 117,480 |
| Clone/apply, 1,000 cells, no history | 2.397 | 1.953 | 1,454,856 | 1,197,800 |
| Clone/apply, 10,000 cells, no history | 24.117 | 23.705 | 15,514,056 | 11,324,840 |
| Clone/apply, 100 cells, 100 history | 0.320 | 0.300 | 254,136 | 226,456 |
| Clone/apply, 100 cells, 1,000 history | 2.156 | 2.018 | 1,277,960 | 1,250,280 |
| Projection, 1,000 cells, 0 pending | 0.411 | 0.422 | 433,152 | 433,152 |
| Projection, 1,000 cells, 1 pending | 2.314 | 2.092 | 1,451,088 | 1,194,032 |
| Projection, 1,000 cells, 8 pending | 12.878 | 13.584 | 8,576,640 | 6,520,192 |

Allocation savings are repeatable: approximately **67.5%** for the 10,000-cell
hash, **27.0%** for that clone/apply workload, and **24.0%** for eight-pending
projection. Timing ranges are broad and overlapping. In particular, the mixed
suite's eight-pending projection median was 5.5% slower, so it must not be omitted
or presented as a general speedup. Desktop responsiveness and service capacity
remain unmeasured.

An isolated follow-up used only zero/eight-pending projection, a 1-second target,
one excluded warmup pair and five alternating measured pairs. The unchanged
zero-pending path measured 0.481 to 0.476 ms/op (-0.9%); eight pending measured
17.492 to 14.014 ms/op (-19.9%). Its trial ranges were 13.837–19.837 ms for control
and 13.144–17.902 ms for candidate. Both the favorable isolated result and the
unfavorable mixed-suite median are retained in `projection-summary.json` and
`summary.json`; the sign change limits any broad latency conclusion. Retain the
streaming implementation for its repeatable byte reduction and verified protocol
compatibility, with representative desktop/service latency qualification open.

The control and candidate executables both passed `TestPerformanceWorkloads`
after measurement, with identical fixture hashes:

| Cells | Canonical hash |
| ---: | --- |
| 100 | `b75c95a4f89e4c8d823eac2b0bdcdf37e1c1042b0c80a589695aef608b9effc0` |
| 1,000 | `31f3bd017a56e4c81f19d1d47b57e610a7de74d41c20377b41a6f63691601862` |
| 10,000 | `ac8ddd4b8e3ae97e33609bf6ec9649fd673d030051975d24b56ff7541fe9e47c` |

## Remaining performance work

**Follow-up:** the later [selection/projection pass](2026-09-06-selection-lifecycle-and-projection.md)
replaces the repeated `Visible` application path described below and repairs
the reproduced drag identity/background/cancellation defects. This earlier
report and its measurements describe the preceding streaming-hash baseline.

Fresh source review confirms `Projection.Visible` still clones acknowledged
state and `applyOperation` clones/indexes the map and validates/hashes it for
each pending operation. Streaming reduces each hash's temporary bytes but does
not remove this repeated work. Next measure an immutable projection/index
strategy while retaining exact pending requests, preconditions, rejection
semantics, failure isolation and caller-owned snapshot isolation. Continue the
repo-wide workplan's persistence, parser, rendering, cache, lifecycle and load
measurement stages; this component optimization does not close those gates.
