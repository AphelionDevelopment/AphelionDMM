# Engine copy repair implementation plan

> Execute inline with the established test-first and verification workflow. No
> subagents or Git integration are authorized by this plan.

**Goal:** Reduce repeated copying of unchanged engine data while preserving
independent document branches, exact canonical bytes and durable publication.

**Architecture:** Imported snapshot state and accepted operation values remain
private and immutable. Public inputs/results continue to be deep copies. Each
branch owns its mutable history maps; validation copies the tile table before
replacing or appending any tile. Clone may share immutable snapshot and accepted
payload values between independently mutable documents.

**Stack:** Go 1.25.13; existing model/engine/server APIs; no new dependencies.
**Spec:** `docs/superpowers/specs/2026-08-24-multiplayer-design.md`, operation and
durability requirements; performance follow-up section 2.

## Constraints

- Do not change canonical bytes, protocol, schema, retained history or actor undo.
- Keep public snapshots, accepted results and inverse operations caller-owned.
- A failed validation or append cannot change published state or another branch.
- Preserve unrelated edits and protected build/release infrastructure.
- Report component timings separately from service, storage and desktop latency.

## 1. Establish current evidence

Files: new `internal/aphelion/collab/engine/copy_performance_test.go` and
`copy_isolation_test.go`; new server `copy_isolation_test.go`; strengthen the
existing failed-append assertion in server `document_test.go`.

- [x] Run branch/result/inverse isolation, concurrent independent branch edits,
  sparse append and rejected-batch tests before production edits.
- [x] Run `TestDocumentCloneAllocationDoesNotScaleWithMapPayload`; record the
  current allocation growth as the failing resource regression.
- [x] Build `engine-control.exe` from the current source, preserve the changed
  control source alongside its binary,
  and retain `TestDocumentCopyWorkloadHashes` output for all eight fixtures.
- [x] Profile Clone/Validate/CloneApply separately at 10,000 cells, no history,
  one level and one change; keep profiling runs out of timing comparisons.
  The benchmark fixtures cover 100/1,000/10,000 cells, 0/100/1,000 retained
  operations, one/five Z levels and one/100 changed cells, including the combined
  10,000-cell/1,000-history/five-level/100-change case.

## 2. Share immutable data inside the engine

File: `internal/aphelion/collab/engine/document.go`.

Interfaces stay `Clone() *Document`, `Apply(Operation, time.Time)` and
`Snapshot() Snapshot`. The implementation change is:

```go
// Clone owns its metadata maps and shares immutable payloads.
snapshot: document.snapshot,
// While copying accepted map entries:
clone.accepted[operationID] = accepted

// Validation owns the table it will replace/append into.
candidate := document.snapshot
candidate.Tiles = slices.Clone(document.snapshot.Tiles)
```

- [x] Document internal immutability and recheck every engine/recovery write.
- [x] Apply this narrow change; retain deep copying on public ingress/egress and
  on changed tile states. Run all new regressions and existing conformance.
- [x] Build `engine-candidate.exe` with the identical benchmark/test sources.
  Independently compare workload hashes and accepted revisions with control.

## 3. Compare and qualify

- [x] Run independent warmups, then five alternating control/candidate trials of
  `BenchmarkDocumentCopyStages`, fixed `-test.cpu=1` and `-test.benchtime=100ms`.
  Preserve all samples and report medians/ranges and allocation counts. Reject
  an optimization that changes work/hash outcomes or has a material regression.
- [x] Run engine/server/store/client race suites, all repository checks through
  `task verify`, explicit hidden history/resize/selection/Save regressions and
  the produced parser/loopback smoke. Real PostgreSQL and hosted/human acceptance
  remain separate gates.
- [x] Publish source/binary/fixture hashes, retained measurement evidence and
  remaining engine/history, desktop and database targets in verification docs.

Outcome: [verification and full scope](../../verification/2026-09-06-engine-copy-performance.md).
All eight workload hashes/revisions match. All 24 measured stage cases improve
in median time and allocation across five alternating trials. This closes the
bounded copy repair; granular hash/index/history attribution, representative maps,
retained memory and desktop/database budgets remain in the repo-wide workplan.
