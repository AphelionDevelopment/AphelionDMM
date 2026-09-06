# Search replacement validation and stopping point

Execution stopped at the user's request after this bounded repair and its final
qualification. The wider audit and QoL goal is not complete. Changes remain
uncommitted on baseline `052e1acb02b790641d63466de40c91d56028383b`; preserve the
existing working tree. No protected infrastructure, protocol or dependencies
were changed in this pass.

## Reproduced failure and repair

Direct inspection of `editor/instance_batch.go`, `editor/collaboration.go` and
`mapadapter/capture.go` found that checking target membership and before-states
did not validate a selected replacement. Six fresh row/bulk cases reproduced:

- A prefab with nil variables, or an exposed variable-name list referencing a
  missing value, was installed before after-state capture rejected it. The
  display remained changed and retained captures blocked Search and Save.
- An empty replacement path panicked in the inherited same-base-type comparison
  and left captures behind.

These are controlled malformed-prefab fixtures, not observations of routine
human use. The red log records all six failures. Existing successful tests did
not detect them.

`CommitInstanceBatch` now checks the shared replacement once, after successful
target capture and before installation. It rejects an empty path and runs the
same `CaptureTile` conversion against a copied instance in a temporary tile.
Failed replacement validation releases this batch's unused captures, reports
one error and leaves the display, authority and history intact. The map remains
usable for a corrected retry. This does not add a new DM expression validator:
capturable unknown paths and opaque variable values remain supported.

A later executor snapshot-read failure still retains the valid, already-entered
edit and its Save guard for explicit retry. A new regression confirms that retry
submits once and undo restores the exact before-state. Broader unsent-intent
recovery UX remains open.

## Final qualification

All results below were rerun after the final code changes:

| Evidence | Result and boundary |
| --- | --- |
| Focused regression | All Search and targeted editor tests pass. Six malformed replacement cases preserve state and allow a valid retry. Unknown replacement path/variables survive one accepted operation and undo. |
| Affected race | Owned search, Search UI, app, workspace, pane, canvas, editor, settings and tools pass. The log contains no skipped tests. |
| Actual hidden workspace | All 48 top-level GLFW/OpenGL workspace tests pass under race in 16.121 s. New network coverage confirms invalid replacement sends nothing, corrected replacement is acknowledged once, and its inverse restores the exact hash. Transport/engine are controlled in-process fixtures. |
| Repository/build | `task verify` passes: zero lint issues, contracts, all Go tests, Rust test/fmt/clippy, release parser and actual `dst/StrongDMM.exe` build. Rust has zero unit tests. Default GL tests skip in this gate; the separate hidden-context run supplies that evidence. |
| Produced smoke command | A fresh binary passes parser open/save/reparse, authenticated loopback joins, accepted revisions 1/2, equal client hashes, leave and shutdown. This is not desktop UI or hosted acceptance. |

Pinned Go 1.25.13 windows/amd64 and Rust 1.82.0 GNU were used. Recorded supporting
tools: Task 3.53.1, golangci-lint 2.12.2, GCC 15.2.0. The inherited ImGui C++
`memset` warning remains. All 528 Go/Rust source and dependency files hashed
before qualification were unchanged afterward. Documentation was finalized later.

Artifacts are under `.artifacts/search-after-values-2026-09-06/`, including red,
green, affected-race, repository and smoke logs; source manifest; matched trial
script, raw samples and summary; and binary hashes. The initial benchmark build
failed because its new test called a nonexistent history-target helper; the
corrected build succeeded. That harness error is retained separately and is not
red/green product evidence.

## Dense-tile baseline and validation cost

Five alternating control/candidate pairs each measure four unchanged replacement
batches with 20 iterations: 40 samples and 800 timed calls. The control is the
pre-validation batch implementation with identical benchmark instrumentation.
No build or other test overlapped these trials. Desktop power/scheduling was not
controlled.

Each fixture has one tile with area/turf and the indicated number of matching
objects. The actual editor batch path receives the same current prefab, captures
before/after values and returns through the no-op commit path. Every run checks
unchanged display state, revision/hash, empty history/journal and zero executor
snapshot reads during timing. All 80 logged fixture records agree across both
binaries for their respective sizes. This excludes Search query/table work,
changed-operation submission, network, render updates and GPU work.

| Objects on one tile | Control -> candidate median ms/op | Candidate range ms/op | Control -> candidate median Go B/op | Control -> candidate allocs/op |
| --- | --- | --- | --- | --- |
| 32 | 0.053865 -> 0.049815 | 0.032105-0.098680 | 25,104 -> 25,560 | 139 -> 144 |
| 256 | 0.532245 -> 0.547700 | 0.444485-0.872670 | 194,576 -> 195,032 | 1,035 -> 1,040 |
| 2,048 | 16.138385 -> 20.560080 | 14.032010-21.682670 | 1,557,771 -> 1,557,982 | 8,203 -> 8,208 |
| 4,096 | 59.081525 -> 54.970210 | 51.590375-91.794245 | 3,097,612 -> 3,097,830 | 16,395 -> 16,400 |

All control/candidate timing ranges overlap; this repair makes no speedup claim.
Validation adds five allocations per batch. Source still scans a tile for every
target's membership, then scans again for every replacement. The timings justify
profiling this repeated work next; they do not attribute all cost to those loops
or establish real-map latency. Grouping by tile is proposed, not implemented.
Changed batches, hidden/base objects, duplicates and stable-ID/order equivalence
need their own measurements and regression cases.

SHA-256 identities:

- Control: `22b4416b9aae4a7ab45c0a0c645fcc81b0777b2ecf1b9d7f43836753a2e87e79`.
- Candidate: `c117674e404a7e48435af8d0c0f083df262d01ce0c4647485020fa1050ba37d6`.
- Desktop: `f3e24c49b11fe811ed92f742a0b6a986e05cbeb194a544fb234aba7e59c91623`.
- Smoke: `f6cded859990696720d07db096479377abcabd6d88bd3f56bf07d54f72b37dca`.

The smoke report's revision field is `undefined`; the binary/source hashes
identify this run. Its fixture and output SHA-256 match
`7d044f7165ca68f32d823f463c6810876c91aa13934d4880fd96810469e9c9be`.

## Resume here

The current tree includes Grab movement, rotation (`[` / `]`), mirrors (`H` /
`V`), Alt+Arrow nudges, floating paste, tool keys and the F1 searchable shortcut
reference. Their evidence and remaining interaction limits are in the
[QoL workplan](../superpowers/plans/2026-09-06-editor-qol-workplan.md).
Restart with the rebuilt `dst/StrongDMM.exe` for human acceptance; a desktop launch
and physical interaction session were not performed in this final pass.

Continue only when the user resumes work:

1. Audit bulk-action limits before changing batching semantics. Current source
   defines `engine.MaxTileChanges = 4096` and protocol messages at 1 MiB; hosted
   configuration may lower the transport limit. Test too-large operations, late
   rejection and preservation of inspectable intent. Do not silently truncate
   or split one logical action into unrelated undo entries.
2. Profile dense-tile membership, capture, deletion/regeneration and replacement
   separately. Compare a tile-grouped candidate against exact ordered/stable-ID
   results and local/network undo, including hidden and base objects.
3. Resume the [repo-wide performance workplan](../superpowers/plans/2026-09-06-admm-performance-followup.md):
   real-map input-to-visible latency, SQL append/recovery, fanout/slow clients,
   parser/FFI/icon lifetime, GPU/idle/reload resource use and retained history.
4. Keep PostgreSQL, hosted CI, external integrations, Meridian map acceptance
   and human usability as separate unrun gates. Rebinding and broader editor
   QoL remain planned; do not describe the whole audit as complete.

Reproduce with Go 1.25.13 on PATH, from the repository root; check each native
exit code:

```powershell
go test ./internal/app/ui/cpsearch ./internal/app/ui/cpwsarea/wsmap/pmap/editor -run '^(TestSearch|TestInstanceBatchRetainsIntentAfterSnapshotFailure|TestChangedGestureRetainsCaptureWhenSnapshotFails)' -count=1
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/aphelion/search ./internal/app/ui/cpsearch ./internal/app ./internal/app/ui/cpwsarea/wsmap ./internal/app/ui/cpwsarea/wsmap/pmap/... ./internal/app/ui/cpwsarea/wsmap/tools -count=1 -v -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
& './.artifacts/search-after-values-2026-09-06/run-trials.ps1'
```

The trial script requires its preserved control/candidate binaries. Rerun gates
after source changes; do not reuse these timings as a new checkout's baseline.
