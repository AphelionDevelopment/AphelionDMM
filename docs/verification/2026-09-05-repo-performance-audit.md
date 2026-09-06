# ADMM repository performance audit — 2026-09-05

## Result and evidence boundary

Fresh source inspection covers the runtime domains below, including inherited
StrongDMM code. Repeated component measurements identify expensive map copying,
hashing, projection, and history retention. This is **partial runtime coverage**:
it does not establish interactive desktop performance, GPU behavior, sustained
concurrent service capacity, or production readiness. No performance optimization
was implemented or speedup claimed. Correctness repairs are the baseline here.

The original nine defects and their baseline reproductions remain in
[the code audit](2026-09-05-code-audit.md). Implementation qualification is recorded
separately in [the repair record](2026-09-06-audit-remediation.md). The remaining
experiments are executable work in [the optimization workplan](../superpowers/plans/2026-09-06-admm-performance-followup.md).

## Reproducible baseline

- Source: `18429fc79bf69bb4720cfc92c13d028313438024` plus the uncommitted audit repairs.
- Windows/amd64; Intel Core i7-9750H, 6 cores / 12 logical processors; 34,223,411,200 bytes physical RAM.
- Go 1.25.13, default 12-way scheduler; Rust 1.82.0 GNU; Task 3.53.1; golangci-lint 2.12.2. Benchmarks are ordinary optimized Go test binaries, without race instrumentation. Native parser/graphics libraries are linked, but the measured component functions do not invoke the environment parser or graphics rendering.
- Single laptop session. Audit compiles, race tests, and database tests finished before timing. OS background activity, power management, temperatures, filesystem cache, and antivirus were not controlled. The observed variation must be retained in any interpretation.
- Harness: `internal/aphelion/perfaudit/performance_test.go`; mechanical fixtures use the existing `/obj/foo` test type with one prefab and one explicit variable per cell. Width 10, one Z level; 100 / 1,000 / 10,000 cells. These are synthetic scaling fixtures, not representative station maps.
- Each operation toggles one tile's variable with a fresh valid operation ID. Clone/apply trials start with the same document/history and include the clone plus validated apply. Fixture construction and operation creation are outside the timer. Projection uses compatible edits on different cells; 0 / 1 / 8 pending operations. Export measures `mapadapter.Export` without writing. Parse includes opening and reading an already-created DMM from the warm filesystem cache.
- SQLite uses WAL, `synchronous(FULL)`, 100 cells, and 0 or 100 retained operations. Each timed append gets a fresh database with the same history and a snapshot at its head. Creation, history loading, snapshot writing, and cleanup are outside the timer. Recovery verifies the resulting revision and hash after every measured append. SQLite is embedded in the benchmark process, not a separate service.
- `TestPerformanceWorkloads` checks valid fixture hashes, real mutation rather than a duplicate fast path, clone isolation, all eight visible speculative edits, and exported/parsed grid sizes before measurement. It is a harness gate, not full product acceptance.

Raw evidence is ignored under `.artifacts/performance-2026-09-05/`: `build.log`,
`warmup.log`, `components.log`, `sqlite.log`, `component-summary.json`,
`document.cpu`, `document.heap`, `profile.log`, `cpu-top.txt`,
`allocation-top.txt`, `pilot.log`, and `source-manifest.json`.

The source manifest records repository-relative source paths and SHA-256 hashes
at measurement time. Subsequent source edits only added ownership comments to
two UI import blocks; the harness and measured behavior are unchanged. The manifest
excludes generated runtime output and personal configuration. Identity anchors:

| Artifact | SHA-256 |
| --- | --- |
| Source manifest | `5419e2467a0b01c2a52651a8af70c17c90a1892e97e6af1a4e7f0544e855f9b3` |
| Benchmark harness source | `381110d2170cf073af38604e09d1f536b6db0302f6d2c1720813e0827dea1737` |
| Benchmark executable | `172ba5c0a9dd6dd7f405dc040b48d446482ae97e97d39ed82931b256f752a284` |
| 100-cell snapshot hash | `b75c95a4f89e4c8d823eac2b0bdcdf37e1c1042b0c80a589695aef608b9effc0` |
| 1,000-cell snapshot hash | `31f3bd017a56e4c81f19d1d47b57e610a7de74d41c20377b41a6f63691601862` |
| 10,000-cell snapshot hash | `ac8ddd4b8e3ae97e33609bf6ec9649fd673d030051975d24b56ff7541fe9e47c` |

## Measurements

One untabulated warmup preceded five trials per case. Component trials targeted
500 ms each; SQLite used three measured appends per trial because resetting its
durable history is expensive. Values below are the median of the five trial
means and their full range, **not operation p50/p95 latency**. Allocation counts
are per call from the median-time trial. They are not live heap or process RSS.

| Component and fixture | Median ms/call | Trial range ms/call | Bytes/call | Allocations/call |
| --- | ---: | ---: | ---: | ---: |
| Hash, 100 cells | 0.103 | 0.092–0.109 | 49,576 | 30 |
| Hash, 1,000 cells | 1.198 | 0.908–2.544 | 502,400 | 46 |
| Hash, 10,000 cells | 16.659 | 13.086–21.576 | 6,207,250 | 137 |
| Clone + apply, 100 cells / no history | 0.329 | 0.281–0.449 | 145,160 | 673 |
| Clone + apply, 1,000 cells / no history | 3.395 | 2.980–3.711 | 1,454,861 | 6,091 |
| Clone + apply, 10,000 cells / no history | 28.955 | 25.967–37.671 | 15,514,069 | 60,210 |
| Clone + apply, 100 cells / 100 prior operations | 0.642 | 0.459–0.681 | 254,137 | 1,477 |
| Clone + apply, 100 cells / 1,000 prior operations | 3.270 | 3.236–3.663 | 1,277,962 | 8,681 |
| Visible projection, 1,000 cells / no pending edits | 0.691 | 0.565–0.837 | 433,153 | 3,001 |
| Visible projection, 1,000 cells / 1 pending edit | 2.370 | 2.156–2.438 | 1,451,095 | 6,056 |
| Visible projection, 1,000 cells / 8 pending edits | 18.393 | 17.266–24.565 | 8,576,689 | 27,441 |
| Export, 100 cells | 0.137 | 0.130–0.230 | 36,768 | 140 |
| Export, 1,000 cells | 2.140 | 1.835–2.265 | 551,794 | 1,059 |
| Export, 10,000 cells | 23.760 | 19.871–25.639 | 4,532,375 | 10,230 |
| Parse, 100 cells | 0.211 | 0.200–0.270 | 22,928 | 141 |
| Parse, 1,000 cells | 0.886 | 0.773–1.236 | 299,785 | 1,054 |
| Parse, 10,000 cells | 5.628 | 5.315–9.834 | 2,401,869 | 10,141 |
| Durable SQLite append, no history | 5.752 | 3.987–33.954 | 401,506 | 2,049 |
| Durable SQLite append, 100 prior operations | 9.640 | 7.090–25.426 | 1,563,397 | 14,400 |

SQLite's wide, overlapping ranges and low sample counts do not establish an
elapsed-time history penalty. The additional allocation and validation work is
observable; longer paired runs and I/O attribution are required before assigning
a latency benefit to a proposed store optimization.

A separate 10,000-cell clone/apply CPU and allocation profile sampled 5.74 seconds
of process activity, including Go benchmark calibration and setup. Flat sampled
allocation was 49.26% in `model.CloneTileState` and 27.15% in `bytes.growSlice`.
SHA-256's AVX2 block routine accounted for 12.09% of CPU samples; GC scanning was
also prominent. Inclusive percentages overlap and must not be added. These
profiles identify copying and canonical-hash buffer construction as candidates;
they do not justify weakening validation or removing the existing map hash.

## Repository coverage and remaining measurements

Directory prefixes below include their subdirectories unless a more specific row
applies. Coverage means entry-point and hotspot/lifecycle inspection, not a claim
to have executed every file or verified every platform. No entire domain can be
declared fast from a negative source inspection.

| Domain | Source accounting and current-source observations | Runtime evidence / remaining work |
| --- | --- | --- |
| Startup/environment | `main.go`; `internal/app` lifecycle, configuration and project loading; `internal/dmapi/dm`, `dmenv`, `dmvars`; `third_party/sdmmparser` Go wrapper and Rust environment parser. Native strings are copied to Go and decoded from JSON; environment loading builds trees and explicitly invokes GC. | Native cross-stack build passes. Cold/warm environment parsing, process-start-to-usable-map, native heap and full-project peak memory **unrun**; component map parsing is not environment parsing. |
| Icons/resources | `internal/dmapi/dmicon`, `internal/platform/texture.go`, `internal/rsc`, window fonts, Rust icon parser. Cache retains decoded images/textures until `Free`; texture deletion is deferred to the UI queue. Failed icon lookups are cached. Metadata decode, PNG conversion, and upload are separate costs. | Hidden-context Save test exercises graphics initialization only. First-use/hit timings, repeated reload retention and GPU disposal **unrun**. Resource fonts, images, text and icons are data/assets, not optimization edits. |
| Maps/save/clipboard | `internal/dmapi/dmmap` including DMM/TGM parser, prefab/instance storage and platform atomic replacement; `dmmsave`, `dmmclip`; `collab/mapadapter`; workspace save/create-map and screenshot entry points. Export dictionaries and canonical state operate over all cells; Save also stages and validates. | Repeated export and warm DMM parse measured above; atomic-save and unknown-content package tests plus actual workspace Save exercised. Full editor Save timing, TGM, cold I/O, clipboard and screenshot readback **unrun**. |
| Frame/input/render | `internal/app/window`, `internal/platform` GLFW/GL/shader/key code; `internal/app/render` including brush/buckets/levels/chunks/units; map canvas/camera/overlays; `internal/imguiext` widgets/layout/style/markdown/icons. A 60 Hz loop draws continuously. Resize allocates an upload buffer and recreates a texture. Chunk culling/batching already exist; partial level updates still rebuild layer lists. | No frame-time, GPU timer, draw-call or input-latency series. Idle/minimized, resize, multi-tab, animated-icon and pan/zoom traces **unrun**; FPS cap does not prove spare capacity. |
| Editing/navigation | Map editor/tools/quick edit/settings/tile menu; `internal/app/command`; `dmmsnap`; `cpsearch`, `cpprefabs`, `cpenvironment`, `cpvareditor`. UI refresh copies a compatibility snapshot, rebuilds zones/buckets and persists prefabs. Environment lists use a clipper; search/filter, prefab ordering and chunk deduplication still warrant size sweeps. Remaining UI layout/menu/dialog/shortcut/preferences/changelog/empty workspace packages belong here or the frame row. | Gesture/history/conflict/unknown-variable regression tests pass. Component cloning and projection costs measured; actual drag/fill/selection/undo/search input-to-visible latency **unrun**. |
| Engine/local | `collab/model`, `engine`, `executor`; `server/document.go`. Owner stages a cloned document; engine validation clones a candidate, builds indexes and computes canonical hashes. `Document.Clone` also copies retained operations and revision hashes. | Map-size and history-size clone/apply and hash baselines plus CPU/heap profiles above. Larger multi-Z/variable-heavy/multi-tile workloads remain. |
| Client projection | `collab/client` and `collab/ui` controller/view models/attachment/conflicts/presence. Visible state reapplies pending operations over cloned authority. Immutable submitted intent now survives incompatible remote progress. | 0/1/8 pending-operation scaling measured. End-to-end network-to-desktop-visible latency, reconnect memory and prolonged conflict retention **unrun**. |
| Transport/presence | `collab/server` owner/hub/HTTP/WebSocket/session/security/limits/presence/snapshot scheduling; `collab/protocol`, `compat`; `cmd/apheliondmm-collab`; `collab/load`, loadtest command and pilot fixtures. Durable publication is ordered and bounded; each subscriber receives an operation copy. | Local public-contract pilot is recorded separately below. Concurrent offered-load, slow readers, many documents, simultaneous presence and per-client applied-hash tracking remain. |
| Persistence/recovery | `collab/store` memory, SQLite and PostgreSQL; SQL schemas are migration/data contracts. SQL append reconstructs and validates retained history, including compacted prefix context. SQLite has one connection; PostgreSQL locks the document row during writes and uses consistent recovery reads. | SQLite fixed-history benchmark above; real PostgreSQL 18.6 conformance, contention, connection termination, and backup/restore passed. No PostgreSQL capacity benchmark, disk-latency profile, large-history crash/recovery timing, or 30-minute lifecycle/heap series. |
| Auth/telemetry/integrations/update | `collab/auth`, `telemetry`, `testoidc`; `integration/manifest`, `integration/meridian`; `internal/app/selfupdate`, `internal/req`. OIDC/OTLP add external I/O; MCP serializes bounded requests with a mutex; staging hashes/copies immutable artifacts. Updater buffers a bounded download then hashes/verifies it. | Unit/fixture and race gates pass. Real OIDC/OTLP latency, remote integration throughput, updater peak-memory and actual Content Tools consumer **unrun**. Do not perform an updater replacement as a benchmark. |
| Executables/tooling/support | Desktop `main.go`; commands `collab`, `hosted`, `healthcheck`, `compat`, `doctor`, `loadtest`, `meridian-verify`, `oidc-fixture`, `smoke`; `internal/aphelion/buildcheck`, `smoke`, `perfaudit`; `internal/env`, `util`; `api/collaboration`, `scripts`, `tools`, Taskfiles, CI/deploy manifests, module/lock files. Support code is accounted through callers; contracts/config/assets are not independent hot loops. | Maintained lint/contracts/Go/pinned-Rust/build and real smoke/doctor gates pass. Rust crate reports **zero unit tests**. Local Linux image build and hosted lifecycle with fixture OIDC/OTLP passed after timing; production deployment remains unrun. Go/race/build elapsed time is developer-tool evidence, not product performance. Protected infrastructure was not edited. |

## Load-harness validity

`load.Run` waits until every socket reports the current operation ID before
offering the next operation, then sends all presence updates after editing ends.
Its `P95AcknowledgementMillis` field measures send-to-all-socket-delivery under
serial demand. It does not measure schedule-to-durable-ack or desktop-visible
latency. `readLoadEvents` does not apply operations and compare each client's
resulting map hash. The final server revision/hash check is useful but does not
close that client-convergence gap.

The checked-in pilot exercises 25 sockets, 250 operations, 500 later presence
updates, a 10×10 one-level map, and a 10-operation/second serial target. Repeated
results and limitations are retained in `pilot.log`; this is a compatibility
load probe. All five runs passed its gate and reached revision 250 and the same
server hash `71913e2be13a2206141dd93bc0d4243eef7a47f33a92b7425ce399966189f05f`.
The per-run delivery p95 values were 11.81, 7.92, 5.00, 6.02 and 10.00 ms;
per-run p99 values were 28.03, 25.98, 8.20, 15.70 and 20.72 ms. Test durations
were 26.92–28.02 seconds. No merged percentile was calculated. Harness expansion
and a sustained-concurrency run remain open work.

## Re-run commands

Run from the repository root with the pinned Go toolchain. Check every exit code.
The original measurements used the same precompiled test binary for warmup and
timed trials, with fully quoted dotted test flags on PowerShell. Equivalent Go
entry points are:

```powershell
go test ./internal/aphelion/perfaudit -run '^TestPerformanceWorkloads$' -count=1 -v
go test ./internal/aphelion/perfaudit -run '^$' -bench '^BenchmarkAudit' -benchtime=1x -count=1 -benchmem
go test ./internal/aphelion/perfaudit -run '^$' -bench '^BenchmarkAudit(SnapshotHash|DocumentCloneApply|ProjectionVisible|MapExport|MapParse)$' -benchtime=500ms -count=5 -benchmem
go test ./internal/aphelion/perfaudit -run '^$' -bench '^BenchmarkAuditSQLiteAppend$' -benchtime=3x -count=5 -benchmem
go test ./internal/aphelion/perfaudit -run '^$' -bench '^BenchmarkAuditDocumentCloneApply$/^cells=10000$/^history=0$' -benchtime=3s -cpuprofile=.artifacts/document.cpu -memprofile=.artifacts/document.heap
go run cmd/pprof -top -alloc_space .artifacts/document.heap
go test -tags pilot ./internal/aphelion/collab/load -run '^TestPilotScenarioThroughPublicContracts$' -count=5 -v -timeout 240s
```

The installed Go tool bundle lacks the `go tool pprof` executable; building the
same pinned Go source with `go run cmd/pprof` successfully decoded both profiles.
An initial unquoted dotted-flag invocation failed before running a benchmark;
it is not included in the results.
