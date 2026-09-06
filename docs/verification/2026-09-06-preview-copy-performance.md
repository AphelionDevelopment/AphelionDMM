# Preview restoration allocation audit

Baseline commit: `052e1acb02b790641d63466de40c91d56028383b`, including the current
uncommitted selection/paste repairs. The current source was inspected directly;
the previous feature's timing was treated as a lead, not a verified comparison.
Artifacts are under `.artifacts/preview-copy-audit-2026-09-06/`.

## Finding and repair

Move.Preview restored every captured tile with Instances.DeepCopy, then cleared
visible contents in owned source/destination tiles and rebuilt their display.
Those restored copies were immediately discarded. A preserved control binary's
500-rotation profile attributes about 133 MB of sampled allocations to that
restore call. CPU samples also identify sorting, instance allocation and garbage
collection/scanning as remaining costs. Profiling timings are not benchmark
comparison results.

A new regression reproduced the dependency on overwritten background size:
repeated one-cell previews allocated 13 times with no extra visible background
objects and 269 times with 256 extra objects. After repair both cases allocate
eight times. The test also checks hidden/visible instance order and stable IDs,
mutates displayed instances, and proves cancellation snapshots remain independent.

The only production change is in `internal/aphelion/editing/move.go`. Normal
previews restore passed-over tiles in full, retain independent copies of hidden
instances in owned tiles, and build the visible preview directly from the source.
All visited coordinates still receive sorted render invalidations. Cancellation
and ordinary return-to-origin retain the full restoration path. Capture preflight,
partial-acquisition rollback, journal release, generation/level checks, operation
submission and undo behavior remain under their existing owners.

No protocol, schema, dependency, generated asset or protected infrastructure
changed. The work remains uncommitted. The SDK lacked a built pprof executable;
its bundled Go 1.25.13 `cmd/pprof` source was built into the ignored audit directory,
without changing the SDK or installing another dependency.

## Matched measurements

Five alternating control/candidate pairs, eight workloads per process and 100
timed calls per workload: 80 samples and 8,000 measured calls. Odd trials run
control first; even trials run candidate first. The preserved original move.go
is compiled through a relative-path Go overlay for the instrumented control.
Both binaries use identical fixture/benchmark code. No other agent-driven build
or test ran during the trials; desktop activity and power scheduling were not
controlled.

Fixture identities are deterministic valid UUIDs. Initial map construction,
placement ID creation/fixture normalization, warmup, explicit GC, final inverse
completion and canonical display hashing are outside timing. Rotation validates
all source identities/coordinates/variables and the actual displayed map after a
full turn. Ordinary movement alternates targets on every measured call, includes
hidden instances, validates its displayed map and checks exact cancellation.
Background counts remain bounded by the selected/source/destination cells.

Every sample's display hash matches calibration and the opposite binary for its
workload. Raw logs, hashes, `trials.csv`, `summary.json` and `run-trials.ps1` retain
the comparison. They cover actual model state, not only planning output.

| Workload | Control -> candidate median ms | Median B/op | Allocs/op |
| --- | --- | --- | --- |
| Rotate 1 cell | 0.007050 -> 0.004182 | 2,002 -> 1,930 | 26 -> 24 |
| Rotate 100 cells | 0.138827 -> 0.105790 | 35,651 -> 28,450 | 626 -> 426 |
| Rotate 4096 cells | 7.198128 -> 6.455456 | 1,509,301 -> 1,214,337 | 24,616 -> 16,424 |
| Rotate 2 sparse cells, extent 16 | 0.007172 -> 0.006424 | 2,626 -> 2,554 | 39 -> 37 |
| Rotate 2 sparse cells, extent 256 | 0.004380 -> 0.004430 | 2,626 -> 2,554 | 39 -> 37 |
| Move 1 cell, hidden objects | 0.001557 -> 0.002134 | 976 -> 688 | 22 -> 16 |
| Move 100 cells, hidden objects | 0.093495 -> 0.082189 | 37,999 -> 21,439 | 811 -> 466 |
| Move 4096 cells, hidden objects | 5.131239 -> 3.834013 | 1,350,476 -> 746,827 | 29,351 -> 16,775 |

Allocation savings are the strongest result. Large-case timing ranges overlap:
rotation control 5.610-12.616 ms versus candidate 4.401-7.490 ms; movement control
4.225-8.832 ms versus candidate 3.326-7.244 ms. All five paired dense-rotation
comparisons are faster in the candidate; large movement is faster in four of five.
One-cell movement and the wider sparse-rotation case have worse candidate
medians. Small timing results do not establish a reliable improvement/regression.

Do not compare this table directly with the earlier 40-iteration feature baseline:
instrumentation now validates actual canonical display state, normalizes fixture
IDs and performs setup GC, and the machine's timing varied substantially. These
are model components, excluding real capture conversion, base regeneration,
render bucket rebuild, GPU upload, network work and full input-to-visible time.
No desktop FPS, process-memory, production-capacity or universal speedup is claimed.

## Qualification and provenance

| Gate | Result |
| --- | --- |
| Red/green allocation regression | 13/269 allocations on control; 8/8 on candidate, with independent cancellation snapshots. |
| Preserved control semantics | Existing move, placement, rotation and resize regressions pass against original move.go through the overlay. |
| Owned model | All 20 top-level tests pass under race in 1.167 s, including sparse geometry, hidden IDs, capture failure, cancellation and orientation rules. |
| Actual workspace | All 41 top-level hidden GL tests pass under race in 12.697 s: Grab/paste lifecycle, shortcuts, text/focus, Save, fault recovery, local/network outcomes and undo/redo. No workspace test skipped. |
| Other affected packages | App 2.183 s; pane 1.207 s; editor 2.264 s; canvas 2.296 s; settings 1.177 s; tools 1.223 s, all under race. Overlay/quick-edit/tile-menu have no test files. |
| Repository/build | `task verify` passes zero-issue lint, contracts, all Go tests, Rust test/fmt/clippy, parser release and desktop build. Rust currently has zero unit tests. Default Go skips gated GL tests; separate hidden runs above supply that evidence. |
| Produced command | Fresh parser/loopback smoke passes open/save/reparse, two authenticated clients, revisions 1/2, equal map hashes, leave and shutdown. It does not exercise desktop UI. |

Toolchains remain Go 1.25.13 windows/amd64, Rust 1.82.0 GNU, Task 3.53.1,
golangci-lint 2.12.2 and GCC 15.2.0. The inherited ImGui C++ memset warning remains.
No source changed after the affected/repository gates. The measured candidate
move.go equals the final source; the manifest records source/doc/binary/evidence
hashes and uses relative paths.

SHA-256 identities:

- Original profile control: `a1d7608df4e05b83bead6ed9560640b95fc4eaf6c642f26a5f732cdf14c9dc2a`.
- Instrumented control: `cdc6577d66ee1de35245fbe788bf59c5a23428ae2ad2ea0e92576e0fb82395a6`.
- Instrumented candidate: `d277b492ea96a06904b858e1a6e0ced994d9dfa2329b25f2be94791bf013c31d`.
- Final desktop: `ff457348c364649eea671b5ba41055e0b65b34921866934fc2f2f12e0249764c`.

The smoke report's revision field remains undefined; binary/source hashes supply
its identity. Hidden fixture evidence does not establish physical input usability,
real sprite/map performance, hosted CI or external integration acceptance.

## Reproduction and next work

From the repository root with pinned toolchains, checking each native exit code:

```powershell
go test ./internal/aphelion/editing -run '^TestPreviewAllocation' -count=1 -v
& './.artifacts/preview-copy-audit-2026-09-06/run-trials.ps1'
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/aphelion/editing ./internal/app ./internal/app/ui/cpwsarea/wsmap ./internal/app/ui/cpwsarea/wsmap/pmap/... ./internal/app/ui/cpwsarea/wsmap/tools -count=1 -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
```

Restart with rebuilt `dst/StrongDMM.exe` for human acceptance. Next profile typed
coordinate sorting, candidate/source instance construction, real capture and
bucket rebuild costs using representative maps and input-to-visible timing. Keep
exact restoration, hidden identity/order, unknown variables and one-operation
history as acceptance requirements. Continue the full repo-wide workplan;
PostgreSQL, hosted CI, Meridian integration and human acceptance remain separate
unrun gates. The overall audit/repair/QoL goal remains active.
