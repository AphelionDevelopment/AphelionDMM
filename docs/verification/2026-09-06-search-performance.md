# Search query, table and lifecycle audit

Baseline commit: `052e1acb02b790641d63466de40c91d56028383b`, with the existing
uncommitted repairs. Current search, editor mutation and workspace routing source
was inspected directly. Evidence lives in `.artifacts/search-audit-2026-09-06/`.
This is one completed investigation within the continuing repo-wide workplan.

## Findings and changes

`cpsearch.doSearch` previously called `InstancesFindByPrefabId` once for every
prefab variant returned by an exact type-path query. Every call traversed the
map. The new owned `internal/aphelion/search.ByPrefabIDs` visits the map once and
groups matching instances by prefab ID. It then emits groups in the caller's
original order. Numeric and single-variant queries keep a direct single scan.
Exact-path matching, variant-group order, tile/instance order and instance
pointers remain unchanged. The helper does not maintain a persistent index.

`cpsearch.showResults` previously built every row on every frame. It now uses
ImGui's list clipper with the actual button/frame and table-padding row height.
Focused-result scrolling is computed before clipping, so F3 can still expose an
off-screen result. A result-generation change during a row action ends the table
iteration. Existing Jump, Select, Delete and Replace actions are retained.

Two lifecycle regressions reproduced failures before repair: filtering a result
list left navigation pointing beyond its new end, and Free retained instances in
unused slice capacity. Navigation now resets when results change; Free drops
both result buffers, and filter refresh clears its previous references before
reuse. A query with no current editor returns empty results. The no-editor guard
has a passing focused test but was not separately executed as a red test.

The inherited changes are marked and confined to `cpsearch`; the independent
query algorithm is Aphelion-owned. No protocol, schema, dependencies, generated
assets or protected infrastructure changed. Search navigation and coordinate
navigation already existed; this pass does not claim to introduce them.

## Matched component measurements

Five alternating control/candidate pairs, five workloads per process, 50 timed
iterations each: 50 samples and 2,500 measured calls. Odd trials start with the
control; even trials start with the candidate. Binaries were compiled before
trials with identical benchmark instrumentation. The control preserves the
pre-repair search production files. No agent-driven build or test overlapped the
measurements; other desktop activity and power scheduling were not controlled.

| Workload | Control -> candidate median ms | Median Go B/op | Median Go allocs/op |
| --- | --- | --- | --- |
| Path query, 1 variant | 0.358492 -> 0.388846 | 311,344 -> 311,213 | 25 -> 25 |
| Path query, 16 variants | 1.237994 -> 0.585406 | 281,236 -> 364,559 | 183 -> 188 |
| Path query, 128 variants | 7.363012 -> 0.608814 | 278,246 -> 370,800 | 1,031 -> 1,036 |
| Table, 100 results | 0.674842 -> 0.129214 | 124,953 -> 20,184 | 3,511 -> 575 |
| Table, 5,000 results | 31.372556 -> 0.128638 | 6,283,706 -> 20,312 | 179,773 -> 591 |

All five paired comparisons improve for multi-variant queries and both table
sizes. The one-variant candidate improves in only two of five pairs; its range
0.334-0.586 ms overlaps the control's 0.347-0.384 ms. No reliable timing gain is
claimed for that case. At 128 variants the ranges are 6.979-7.530 ms versus
0.568-0.668 ms; at 5,000 rows they are 30.814-32.690 ms versus 0.123-0.131 ms.

Multi-variant allocation increases by about 83-93 KB/query in these fixtures.
Grouping and flattening need temporary storage, and resetting the query releases
the old result buffer. This tradeoff must remain visible in subsequent work.
Clearing retained references is a reachability result, not a measured reduction
in the desktop's process memory. Go benchmark allocation excludes native ImGui,
GLFW and GPU allocations.

### Workload and equivalence boundaries

Query fixtures have 10,000 cells in one row, one matching instance per cell and
1/16/128 prefab variants. These read-only query fixtures use the real editor but
do not attach valid editing authority or native map rendering. Setup, expected
result scans, warmup, GC and final ordered stable-ID hashing are outside timing.
Existing search logging remains enabled in both binaries. Sparse/no-result,
many-instances-per-tile and representative multi-level maps remain unmeasured.

Each query checks against independent repeated single-ID scans and logs an
ordered stable-ID SHA-256. A separate fixed-seed test (`20260906`) covers 200
maps/query combinations, duplicate/missing IDs and exact instance-pointer order.

Table fixtures execute the actual `showResults` and ImGui Render at a 600x400
window within a 640x480 headless context. They include Go widget construction and
native ImGui draw-data generation, without GPU rasterization or map rendering.
Raw vertex/index buffers and draw-command clip rectangles/counts are hashed
after timing. The fixture also requests the final row and verifies its scroll
position and draw output. Scroll maxima match at 1,935 and 114,635 pixels.

The ImGui window persists between benchmark calibration and measurement. The
post-calibration final-row check leaves the measured steady viewport at the
bottom. Equivalence is checked as the same ordered sequence of query, initial
draw and bottom draw records for every binary/trial; initial and bottom hashes
are not incorrectly required to equal one another. All 14 records per process
match across all ten processes. This is draw-geometry evidence, not pixel or
human-interaction evidence. `run-trials.ps1`, raw logs, `trials.csv`,
`summary.json` and `equivalence.txt` preserve the comparison.

## Qualification

| Gate | Fresh result |
| --- | --- |
| Red/green | Filter-index, retained-reference and off-screen table-work tests fail on the original source and pass after repair. Existing ordering and scroll behavior pass both versions. |
| Owned query | Fixed-seed independent equivalence test passes under race; package 1.302 s. |
| Search UI | All six focused tests pass under race; package 1.235 s, including no-editor, retention, filter, viewport scaling and distant scrolling. |
| Actual workspace | Hidden GLFW/OpenGL workspace suite passes under race in 11.322 s, with no skips. New F3/RightShift+F3 test uses the shortcut registry, wraps across two map levels, and preserves authority hash. |
| Other affected packages | App 1.958 s; pane 1.203 s; canvas 2.121 s; editor 2.007 s; settings 1.158 s; tools 1.172 s under race. Packages without tests remain labelled in the raw log. |
| Repository/build | `task verify` passes: zero-issue lint, contracts, all Go tests, Rust test/fmt/clippy, parser release and actual desktop build. Rust currently has zero unit tests. Default GL skips are covered by the separate affected run above. |
| Produced command | Fresh smoke binary passes parser open/save/reparse and loopback service launch, two clients, revisions 1/2, equal hashes, leave and shutdown. This command does not exercise desktop UI. |

Pinned toolchains: Go 1.25.13 windows/amd64 and Rust 1.82.0 GNU. Recorded tools:
Task 3.53.1, golangci-lint 2.12.2, GCC 15.2.0. The inherited ImGui C++ memset
warning remains. All 521 source/dependency files hashed before repository
verification remain unchanged afterward. Documentation and evidence manifests
were finalized after the gates; no production or test source changed.

SHA-256 identities:

- Control: `c831bc85fb05635d4551b871131f0fd966d7c11e660f5ca1656f7f13268d2a14`.
- Candidate: `41702956a995121eb080c5a68dc18b133e7507d3398eabada463ec22856a5773`.
- Desktop: `ee4b4e4dc39a0c28d6eb46b0c6bf68c945a696f799a3757a74dd2d3b3907dbd0`.

The smoke report still has an undefined revision field; binary and source hashes
identify this run. The manifest uses relative paths. Work remains uncommitted.

## Reproduction and remaining work

From the repository root with the pinned toolchains, checking each native exit:

```powershell
go test ./internal/aphelion/search ./internal/app/ui/cpsearch -count=1 -v
& './.artifacts/search-audit-2026-09-06/run-trials.ps1'
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/aphelion/search ./internal/app/ui/cpsearch ./internal/app ./internal/app/ui/cpwsarea/wsmap ./internal/app/ui/cpwsarea/wsmap/pmap/... ./internal/app/ui/cpwsarea/wsmap/tools -count=1 -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
```

The trial script requires the preserved binaries; source/control copies and
binary hashes are retained alongside it. A new source baseline requires new
matched binaries, rather than comparing later code to these timings directly.
Restart with rebuilt `dst/StrongDMM.exe` for human acceptance.

The later [ownership/capture pass](2026-09-06-search-ownership.md) repairs stale
result references, closed/switched-map actions and capture preflight, and checks
local/network outcomes. Oversized bulk actions and broader UI/performance
acceptance remain open. The following leads were recorded at this audit boundary:

- `Search` retains instance pointers between queries. Workspace switches and
  search actions refresh it, but `refreshCollaborationView` refreshes prefabs and
  variable UI without refreshing Search. Same-editor snapshot replacement or
  resize can therefore leave cached result references/coordinates stale. Actions
  pass them to current-editor methods; deletion/replacement dereference GetTile
  without a nil check. Reproduce old-result actions after shrink, undo and remote
  replacement before selecting generation invalidation or stable-ID resolution.
- Delete All / Replace All still need payload-limit, authority-failure,
  one-logical-action history and large-result qualification. Clipping does not
  make bulk mutation safe or bounded.
- XY bounds filtering has no explicit Z-level selector. Query semantics and
  filter persistence need a reviewed UX choice before extending them.
- Measure real fonts/scales, row-action refresh, focus/popups, representative
  maps, native/process memory and full input-to-visible latency. Current tests
  do not supply manual mouse, keyboard-layout or GPU acceptance.

Continue the [performance workplan](../superpowers/plans/2026-09-06-admm-performance-followup.md)
and [editor QoL workplan](../superpowers/plans/2026-09-06-editor-qol-workplan.md).
PostgreSQL, hosted CI, external integrations and human acceptance remain separate
unrun gates. The overall audit/repair/QoL goal remains active.
