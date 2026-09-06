# Concurrent collaboration load measurement

This pass repairs a measurement gap found by reading the current load runner.
The legacy pilot waits for all clients after each operation and publishes
presence afterward. Its acknowledgement fields measure observation by all
clients; its readers do not apply accepted state. These limitations prevented
it from establishing concurrent service capacity. Its output and default mode
are preserved for compatibility; its reader cancellation and cleanup are fixed.

The new explicit `-concurrent` command mode uses the same authenticated HTTP and
WebSocket service routes. It adds independent absolute operation schedules,
simultaneous presence, deliberate conflicts, independently applied client state,
separate latency distributions, and failed-run JSON with partial counters.
It is measurement infrastructure. No service speedup or capacity claim follows
from its correctness tests.

## Source and workload contract

- Owned implementation: `internal/aphelion/collab/load/concurrent_*.go` and
  `cmd/apheliondmm-loadtest/concurrent.go`; narrow command routing in `main.go`.
- Recorded workload: `testdata/collaboration/load/concurrent.json`, seed
  20260906, four clients, 40 operations, eight conflict pairs, 32 distinct cells
  on an 8-by-4 map, 80 offered operations/second, ten presence updates per client
  offered at 20/second. The 250 ms schedule-to-ack p95 threshold is a configured
  experimental gate, not an established product SLO.
- Every intent has the same authenticated revision-zero base. Independent
  intents use distinct cells. Both members of a conflict pair request identical
  content with different operation IDs, so exactly one wins regardless of
  arrival order: 32 acceptances, eight rejections, one expected canonical hash.
  Randomized engine replay checks the analytical manifest independently.
- Each client applies every accepted event through `client.Projection.Accept`,
  validating contiguous revision, accepted content and canonical hash. The
  final HTTP snapshot must match every client and the manifest. Unexpected
  operations, rejections, sessions, stream corruption or disconnects fail.
- The overall deadline is mandatory. Readers and writers stop before result
  collection returns. Completion accounting is constant work per event;
  manifest-sized bitmasks keep delivery accounting bounded by operation count.
- Scenario decoding rejects unknown fields, trailing JSON, oversized input and
  impossible cell/conflict counts. Credentials continue to come from the owner
  environment variable and optional hosted credential file, never command-line
  arguments or result JSON.

## What the fields measure

Connection setup and final HTTP verification are excluded from latency samples.
The shared epoch is fixed after all clients join. No acknowledgement releases
the next operation. Slow socket writes retain absolute schedule lag rather than
shifting the offered schedule. Clients serialize their own WebSocket writes.

| Field | Boundary |
| --- | --- |
| `schedule_to_acknowledgement` | Scheduled intent time to sender receipt of a validated accepted event; includes local scheduling and write backlog. |
| `send_to_acknowledgement` | Immediately before envelope encoding/write to sender receipt; includes encoding and socket write wait, excludes earlier write-mutex wait. Receipt is timestamped before decoding/application, but counted only after successful application. |
| `send_to_all_applied` | Same send start to completion of validated application by the last client. |
| `send_to_rejection` | Same send start to sender receipt of an expected, authority-checked precondition rejection. |
| `planned/scheduled/started/sent_operations` | Manifest size / intents due by cutoff / writes started / writes returned successfully. Failed sends can have unknown server outcomes. |
| `accepted_operations`, `applied_deliveries` | Unique accepted intents observed by at least one client / unique validated client applications. On failure, an acceptance can lack its sender acknowledgement. |
| `unresolved_operations` | Started operations without a validated sender acceptance or rejection. |
| `unsent_backlog`, `peak_send_backlog` | Scheduled intents without successful write completion at cutoff / maximum such count, including in-progress writes. |
| `achieved_accepted_per_second` | Observed accepted count over epoch to last sender outcome. Interpret only with outcome/application counts and `gate_passed`; partial failure rates cannot establish capacity. |
| Presence fields | Planned updates, successful sends, local negotiated-rate coalescing, unique observed receiver deliveries, and unobserved deliveries at the bounded cutoff. Sender delivery is included. |

Latency summaries contain sample counts, nearest-rank p50/p95/p99 and maximum.
The p95 gate uses schedule-to-ack. Presence is locally coalesced to the advertised
publication interval and observed for one maximum advertised interval plus 25 ms
after operations/writers finish. Missing presence may be coalesced, delayed or
unobserved; it is not identified as packet loss. Failed runs may retain presence
updates neither sent nor locally coalesced, so planned and completed counts need
not balance. The protocol's configured persistence still determines what an
acknowledgement guarantees.

## Verification

Current raw artifacts are under `.artifacts/concurrent-load-2026-09-06/`.
Focused tests cover deterministic conflict outcomes, absolute schedules, latency
attribution, send/reader completion races, malformed manifests, cancellation,
partial failure output and real HTTP/WebSocket client application.

A controlled WebSocket proxy withholds every operation outcome until all nine
edits and at least three presence messages have been forwarded. A serial runner
cannot pass. Separate tests stall one client and corrupt an accepted hash; both
must fail with partial delivery counts rather than report convergence.

The command gate creates a disposable SQLite-backed service, sends nine intents
with two expected conflicts from three clients, checks 21 validated deliveries,
then shuts down and reopens SQLite and restores the same revision-seven hash.
`TestConcurrentProducedBinary` additionally executes the supplied built command;
its default skip is not executable evidence. This is a small correctness fixture,
not the recorded 40-operation performance scenario or a saturation experiment.

Final gate results:

| Evidence | Result |
| --- | --- |
| Focused load and command tests | Passed, including proxy-held outcomes, stalled/corrupt streams and SQLite reopen. |
| Affected load and command race tests | Passed in 5.187 s and 1.665 s respectively. |
| Maintained `task verify` | Passed: lint, contracts, all Go packages, pinned Rust test/fmt/clippy, parser release build and Windows desktop build. The Rust crate still reports zero unit tests. |
| Produced command | Five fresh disposable SQLite runs passed; nine sent, seven accepted, two expected rejections, 21 validated client applications, no unsent or unresolved operations in each. |
| Durable reconstruction | SQLite 3.53.3 reopened successfully after each run; restored revision seven and the same hash as all clients and the final HTTP snapshot. This is orderly reopen, not crash/power-loss evidence. |
| Presence observation | Each run sent all nine planned updates; unobserved receiver deliveries at cutoff were 5, 3, 2, 2 and 10 out of 27 possible. These are retained rather than reclassified as successful delivery. |

The initial repository gate stopped on three unchecked test cleanup errors;
those were fixed and the entire gate rerun successfully. Both logs are retained.
The inherited ImGui `memset` compiler warning remains. Default-skipped hidden
OpenGL, PostgreSQL and external tests are not converted into fresh acceptance by
the repository gate. No desktop code changed in this pass; the newly built
desktop hash matches the previous selection/nudge pass.

Provenance: Go 1.25.13 Windows/amd64, Rust 1.82.0 GNU, Task 3.53.1,
golangci-lint 2.12.2, uncommitted working tree based on
`052e1acb02b790641d63466de40c91d56028383b`.

- Load-test binary SHA-256:
  `bf9ba3c5e836bf0de9c672d32d9d9507aaaa34f6ad567047227c565a7f9c272c`.
- Desktop binary SHA-256:
  `b9118d92cca3577ac3e7c56215358afe0f87f587c86ca040314edfe4db7a0ebf`.
- Every command trial's final map hash:
  `6a634e41d0660f3d9a593e008f8f73f0bcd8974d711ecd269bc895c09eefb452`.
- Raw files: `focused.txt`, `focused-race.txt`, `verify-lint-failed.txt`,
  `verify.txt`, `produced-binary.txt`, `produced-results.json`, and `manifest.json`
  in the artifact directory above. The manifest hashes load/command source files
  and records the separate nine-operation command fixture and binary hashes.

Timing samples from this tiny correctness fixture are not used to claim capacity.
Some sub-millisecond rejection measurements read as zero on this Windows host;
the output retains them and their sample counts. Longer calibrated trials and
timer-resolution characterization remain part of the performance campaign.

## Reproduction and next work

Use the repository-pinned Go and Rust toolchains, then from the repository root:

```powershell
go test ./internal/aphelion/collab/load ./cmd/apheliondmm-loadtest -count=1 -timeout 60s
go test -race ./internal/aphelion/collab/load ./cmd/apheliondmm-loadtest -count=1 -timeout 90s
task verify
$loadEvidence = Join-Path $PWD '.artifacts/concurrent-load-2026-09-06'
New-Item -ItemType Directory -Force $loadEvidence | Out-Null
go build -trimpath -o (Join-Path $loadEvidence 'apheliondmm-loadtest.exe') ./cmd/apheliondmm-loadtest
$env:APHELIONDMM_LOADTEST_BINARY = Join-Path $loadEvidence 'apheliondmm-loadtest.exe'
go test ./cmd/apheliondmm-loadtest -run '^TestConcurrentProducedBinary$' -count=5 -v -timeout 90s
Remove-Item Env:APHELIONDMM_LOADTEST_BINARY
```

Check each native exit code. For a separately provisioned controlled service,
create a fresh session from `load.GenerateConcurrent(config).Initial`, retain its
owner credential in `APHELIONDMM_LOAD_OWNER_TOKEN`, and run:

```powershell
& (Join-Path $loadEvidence 'apheliondmm-loadtest.exe') -concurrent `
  -endpoint $loadEndpoint -origin $loadOrigin -session $loadSession `
  -scenario testdata/collaboration/load/concurrent.json -timeout 2m
```

The initial snapshot/document ID must match the selected deterministic manifest;
each trial needs a fresh matching session. Preserve stdout even when exit code is
nonzero. Hosted mode still requires one distinct authorized editor credential per
client. No hosted credentials, deployment or protected infrastructure changed.

Remaining gates: record representative map/history/batch sizes, repeat controlled
offered-rate sweeps with warmup and at least five trials, and profile the client
driver separately from the service and database. `Projection.Accept` deliberately
does real client clone/hash/application work; driver CPU can limit a shared host.
Add slow-client disconnect/recovery, reconnect and ambiguous-send accounting,
multi-document contention, durable fault/restart and long-history campaigns.
Real PostgreSQL, separate server-process load, hosted acceptance, desktop/GPU
timing and human tool acceptance remain separate workplan items.
