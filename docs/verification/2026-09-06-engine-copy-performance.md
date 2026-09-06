# Engine copy performance and isolation verification

This pass inspected current engine mutation, inverse, recovery and server
publication code in the uncommitted tree based on
`052e1acb02b790641d63466de40c91d56028383b`. The preserved control already includes
the preceding streaming-hash, projection and editor repairs. Older audit timings
are not the baseline for this comparison.

## Change and ownership contract

The server's `submit` clones a document before applying an operation and appending
its accepted record. Previously, that clone copied every tile payload and retained
accepted operation, and validation copied the entire snapshot again. The new
`Document.Clone` shares private immutable snapshot/accepted payloads and copies
the mutable revision, accepted-record and inverse maps. Validation takes its own
tile table before replacing existing entries or appending new ones.

NewDocument inputs, operation inputs, changed tile states, public Snapshot,
accepted results and inverse results retain their deep-copy boundaries. Recovery
deep-copies imported accepted records; its temporary validator only reads shared
history maps. No existing private prefab slice, variable map or accepted payload
is mutated in place. Future engine changes must preserve this contract. Separate
branches can be edited independently; a single Document still requires its
existing serialized owner. This does not make concurrent Apply/Clone on the same
Document safe.

The production edit is confined to
`internal/aphelion/collab/engine/document.go`. Server publication still occurs
after append succeeds or a committed-then-error outcome is reconciled. Protocol,
schema, canonical bytes, SHA-256, retained history and actor undo are unchanged.

## Regressions and control

Before the production change, new isolation and failed-append tests passed. The
new allocation-growth regression failed: Clone required 306 allocations for
100 cells and 30,006 for 10,000 cells. It now reports five allocations for both.

New tests exercise independent concurrent branches, mutation of returned
snapshots/accepted/inverse values, actor inverses on every branch, rejected
multi-tile batches after a valid earlier change, invalid resulting stable IDs,
and sibling appends into a sparse map. The server regression attempts to change
an existing tile, fails append and mutates the caller's operation afterward;
the published snapshot remains exactly equal. The existing failed-append test
also checks hash as well as revision.

All eight deterministic workloads produce identical base hashes, result hashes
and accepted revisions in separately built control and candidate test binaries.
The expected resulting snapshot is constructed from the fixture and requested
changes without calling engine validation/application. Every timed Validate and
CloneApply iteration checks its result hash; CloneApply also checks revision.
The hash implementation is common to both binaries, with its independent
canonical compatibility tests included in the repository gate.

## Matched component measurements

Environment: Go 1.25.13 windows/amd64; Intel Core i7-9750H at 2.60 GHz;
`-test.cpu=1`. Each binary received an independent 50 ms warmup. Five 100 ms
control/candidate trials alternated order, using the same binaries, fixtures,
benchmark code and correctness checks. Compilation, tests and profiling did not
run alongside the comparison. Fixture/history construction is outside each
measured child stage. All 240 measurement rows are retained.

Fixtures contain one `/obj/foo` prefab with one `dir` variable per explicit cell.
History repeatedly toggles the first cell. A measured operation changes the
first one or 100 cells; the five-level fixtures include unchanged levels, and
the changed cells are on the first level. This is a sparse matrix of synthetic
workloads, not every combination or a representative game map.

The table reports CloneApply medians and observed minimum/maximum in milliseconds;
byte and allocation columns are per-operation medians.

| Cells / history / Z / changed | Control ms [min, max] | Candidate ms [min, max] | B/op control → candidate | Allocs/op control → candidate |
| --- | --- | --- | --- | --- |
| 100 / 0 / 1 / 1 | 0.134 [0.123, 0.152] | 0.068 [0.064, 0.070] | 117,480 → 35,816 | 667 → 66 |
| 1,000 / 0 / 1 / 1 | 1.765 [1.627, 1.997] | 0.801 [0.701, 0.905] | 1,197,800 → 380,648 | 6,082 → 81 |
| 10,000 / 0 / 1 / 1 | 18.169 [17.024, 18.679] | 8.276 [7.775, 8.594] | 11,324,840 → 3,161,512 | 60,197 → 196 |
| 100 / 100 / 1 / 1 | 0.266 [0.241, 0.294] | 0.105 [0.092, 0.118] | 226,456 → 59,992 | 1,471 → 170 |
| 100 / 1,000 / 1 / 1 | 1.806 [1.555, 1.854] | 0.410 [0.385, 0.510] | 1,250,280 → 320,616 | 8,675 → 1,074 |
| 1,000 / 0 / 5 / 100 | 2.148 [1.856, 2.195] | 1.167 [0.897, 1.216] | 1,493,280 → 676,128 | 8,166 → 2,165 |
| 10,000 / 0 / 5 / 100 | 17.236 [17.040, 34.925] | 9.043 [8.114, 9.484] | 11,620,320 → 3,456,992 | 62,281 → 2,280 |
| 10,000 / 1,000 / 5 / 100 | 20.157 [19.990, 25.121] | 9.670 [9.370, 10.781] | 12,753,120 → 3,741,792 | 70,289 → 3,288 |

All 24 Clone/Validate/CloneApply cases have lower candidate median time, bytes
and allocation counts. The measured time ranges do not overlap between roles
within any case. The 34.925 ms control outlier remains in the results. Five short
trials on one laptop do not establish a portable latency budget or statistical
confidence interval.

Without retained history, Clone now allocates 496 B in five allocations at every
tested map size. With 1,000 retained operations it still allocates 285,504 B in
1,014 allocations: history-map copying remains proportional to history size.
Validation still copies/indexes the tile table, validates the whole map, sorts
for canonical encoding and hashes all canonical bytes. CloneApply at 10,000
cells/one change is about 54% faster with 72% fewer allocated bytes in this
component experiment. No desktop frame, service throughput, database latency,
RSS/private-byte or GPU improvement is claimed.

## Profiles and next targets

Separate Clone, Validate and CloneApply CPU/allocation profiles were collected
for control and candidate at 10,000 cells, no retained history, one level and one
changed cell. Profiling uses a 4 KiB allocation sample rate and a one-second
benchmark target; its throughput and total allocation are excluded from the
comparison. Profiles include setup/calibration and different iteration counts,
so aggregate sampled megabytes cannot be compared as per-operation costs.

In the control's separate Clone profile, CloneSnapshot accounts for 98.73% of
sampled allocated bytes. In the candidate CloneApply profile, Snapshot.Validate
accounts for 47.21% flat allocation, the validation function for 19.99%, Hash for
15.07%, and the tile-table clone for 14.73%. SHA-256's AVX2 block routine accounts
for 31.55% of sampled CPU; Snapshot.Hash is 80.10% cumulative CPU. These are
profile attribution, not additional speedup measurements.

Next inspect coordinate/stable-ID index construction, repeated validation and
canonical sort/encoding separately. Preserve full validation of public inputs,
global stable-ID uniqueness, historical-base checks and exact canonical bytes.
History-map attribution with long histories remains open, as do retained-state
memory and real map density/variable/instance diversity. Database prefix replay,
actual desktop editing and slow-consumer recovery remain separate workplan tasks.

## Verification and provenance

Raw evidence is in `.artifacts/engine-copy-audit-2026-09-06/`:

- `document-control.go.txt`, `engine-control.exe`, `engine-candidate.exe` preserve
  the changed control source and both measured binaries.
- `allocation-red.txt`, `isolation-before.txt`, `append-before.txt`,
  `engine-green.txt`, `append-green.txt` record narrow red/green evidence.
- `control-hashes.txt`, `candidate-hashes.txt`, `matched-workload-hashes.txt`
  retain the eight workload comparisons.
- `compare.ps1`, warmup/trial logs, `samples.csv`, `comparison.json` and
  `comparison-summary.txt` retain methodology and all 24 stage results/ranges.
- `profile.ps1`, six stage CPU/allocation profile pairs and top reports retain
  diagnostic attribution. The pinned SDK lacked a prebuilt `go tool pprof`;
  `pprof.exe` was built from that SDK's existing `cmd/pprof` sources. An initial
  unsupported report flag was corrected; the successful reports are retained.

| Gate | Observed result |
| --- | --- |
| Engine package | All tests passed in 13.898 s, including allocation-growth, branch isolation, conformance, rejection, idempotency and inverse cases. |
| Failed append after change | Both focused server regressions passed; existing tile contents, revision and hash remain authoritative. |
| Affected race suites | Model 2.838 s, engine 100.034 s, server 3.920 s, client 3.893 s, store 1.068 s and SQLite 1.417 s; all passed. PostgreSQL package tests passed in 1.148 s, but database-dependent cases were skipped. |
| Maintained repository gate | `task verify` passed lint (zero issues), contracts, all Go tests, pinned Rust test/fmt/clippy, parser release build and Windows desktop build. Rust has zero unit tests. |
| Hidden workspace under race | All 25 top-level history, resize, selection and Save tests passed in 7.556 s with `APHELIONDMM_GL_TEST=1`; no hidden-context skip. |
| Produced parser/loopback smoke | Parser open/save/reparse, authenticated two-client collaboration, revisions 1/2, equal client hashes, leave and natural shutdown passed. |

The inherited ImGui C++ `memset` warning remains. Go is 1.25.13 windows/amd64,
Rust 1.82.0 GNU, Task 3.53.1, golangci-lint 2.12.2 and GCC 15.2.0. SHA-256:

- Control test binary: `b0e01b704b2833556ab26f4e6764c64fd2e006d5d2ae4596127ccd54c9821fc4`.
- Candidate test binary: `d18dca5c3c5d825696bb54fc7fd217c3acd70ab619b180cf44cd75359c49e7b1`.
- Desktop: `89a82f769c90ac362ee53e4acdffb8cf0f37ad2abc5090e3affa559e79fad5a6`.
- Smoke command: `f6cded859990696720d07db096479377abcabd6d88bd3f56bf07d54f72b37dca`.
- Smoke input/output: `7d044f7165ca68f32d823f463c6810876c91aa13934d4880fd96810469e9c9be`.
- Both smoke clients: `198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa`.

`manifest.json` records relative source, document, binary and evidence paths with
hashes, toolchains and workload hashes. The control differs from the candidate
only in the preserved engine production source. The smoke reports revision
`undefined`; its binary hash identifies it. No Go/Rust source changed after the
gates. Documentation and the manifest were completed afterward.

Live PostgreSQL/backup tools were not configured. Hosted CI, production load,
Meridian integration, representative desktop/GPU measurements and human
interaction acceptance were not run. No Git integration, protected infrastructure
change, protocol/schema change or external deployment occurred. The broader
repo-wide audit and feature workplan remain open, including partial selection
capture recovery and placement preview.

Reproduce with the pinned toolchains from the repository root, checking every
native command's exit code:

```powershell
go test ./internal/aphelion/collab/engine -count=1 -timeout 120s
go test ./internal/aphelion/collab/server -run 'Test(FailedAppendPreservesExistingTileState|DocumentOwnerDoesNotAdvanceWhenAppendFails)$' -count=1 -timeout 60s
go test -race ./internal/aphelion/collab/model ./internal/aphelion/collab/engine ./internal/aphelion/collab/server ./internal/aphelion/collab/client ./internal/aphelion/collab/store/... -count=1 -timeout 300s
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/app/ui/cpwsarea/wsmap -run '^Test(History|Resize|Selection|SaveAcknowledgementBoundaries)' -count=1 -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
go build -o .artifacts/engine-copy-audit-2026-09-06/smoke.exe ./cmd/apheliondmm-smoke
& ./.artifacts/engine-copy-audit-2026-09-06/smoke.exe
```
