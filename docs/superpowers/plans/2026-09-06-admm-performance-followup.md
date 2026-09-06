# ADMM performance follow-up workplan

The [September 5 measurements](../../verification/2026-09-05-repo-performance-audit.md)
rank the component costs below. This plan is the output of the performance audit;
its optimizations have **not** been implemented. Establish representative workload
and desktop/service evidence before accepting a production performance claim.
Preserve the correctness repairs, pinned toolchains, source ownership markers,
unrelated dirty work, and the protected-infrastructure approval boundary.

## 1. Close the measurement gaps

- [ ] Extend the owned load harness in `internal/aphelion/collab/load` with tests
  that fail for serial offering, sequential presence, lost reader cancellation,
  missing client application, and incorrect latency attribution. Retain the
  existing pilot explicitly as a compatibility scenario.
- [ ] Generate deterministic independent concurrent intents, rather than sending
  the current revision-dependent operation sequence concurrently. Include
  deliberate same-tile conflicts with exact expected accepted/rejected counts.
  Apply every received accepted event to each client's projection and verify
  revision/hash equality. A slow reader must either converge or be explicitly
  disconnected and recovered; dropping work cannot improve reported throughput.
- [ ] Track scheduled, sent, accepted, rejected and observed counts, achieved
  rate and backlog. Report schedule-to-durable-ack, send-to-durable-ack and
  all-client-delivery separately. Send presence concurrently and count drops.
- [ ] Inventory approved local DME/DMM/TGM fixtures with hashes, cell/level/type,
  prefab and variable counts. Add typical and large maps to the current synthetic
  100/1,000/10,000-cell matrix. Keep fixtures outside published artifacts when
  they contain private content; record repository-relative identifiers.
- [ ] Add UI-thread stage timings around gesture capture, operation dispatch,
  projection application, `refreshCollaborationView`, bucket rebuild and next
  displayed frame. Instrument native parser transfer, icon decoding and upload
  separately. Use a maintained owned diagnostic wrapper; any necessary protected
  entry-point change must first receive exact-file approval.
- [ ] Run a fresh-process/warm-cache matrix for startup, DME reload, first/cached
  icons, DMM/TGM open/save, drag/fill/paste, undo/redo, search, pan/zoom, level
  switch, resize, multiple tabs, and idle/minimized states. Capture CPU, Go heap,
  native/private bytes, GPU memory/timers and input-to-visible percentiles.
- [ ] Run a 30-minute repeated open/edit/save/reconnect/close lifecycle workload
  and compare post-GC Go heap, native process memory, goroutines, handles and GPU
  resource counts at stable checkpoints. Capture long-history restart, crash,
  database loss and slow-consumer scenarios without disabling durability.

**Gate:** fixed manifests, correct counts/hashes, independent warmup, at least five
trials, retained raw results, explicit failure/timeout counts and no hidden
closed-loop reduction of offered demand. Desktop/GPU, service, database and
developer-tool results remain separate. These experiments remain open; this
audit's component results do not substitute for them.

## 2. Reduce engine copy and canonical-hash allocation

**Priority:** highest measured component target. On the synthetic 10,000-cell
fixture, one clone/apply allocated 15.5 MB in about 60,210 allocations and took
25.967–37.671 ms per call across trial means. Tile-state cloning represented
49.26% of sampled allocation; buffer growth represented 27.15%.

**Files:** `collab/engine/document.go`, `model/clone.go`, `model/hash.go`, and
`server/document.go`, all under `internal/aphelion/`.

- [ ] Attribute the owner clone, validation clone/indexing, canonical encoding,
  SHA-256 and history-map copies separately with the existing profile baseline.
- [ ] First prototype sharing immutable unchanged tile states or avoiding a
  redundant transaction copy. Keep accepted and caller-owned snapshots isolated,
  and do not publish any state before append succeeds. Do not replace the wire
  hash algorithm or change canonical bytes to obtain a benchmark gain.
- [ ] If encoding buffer reuse is justified, prove lifetime isolation, bounded
  capacity and concurrent safety. Streaming canonical bytes into the same hash
  is a candidate only if it reduces measured allocation without regressions.
- [ ] Add mutation-isolation and failed-append tests before behavioral changes.
  Re-run engine/hash conformance, inverse/recovery/corruption tests, race checks,
  and actual server acceptance with the same revision/hash outcomes.
- [ ] Compare interleaved control/candidate trials at 100/1,000/10,000 cells,
  multiple Z levels, 1/large tile batches and 0/100/1,000 retained operations.
  Accept only a repeatable allocation/time improvement with unchanged work and
  no material regression in another fixture; set a user-facing budget only after
  the desktop/service timings from step 1 exist.

## 3. Bound repeated speculative projection work

**Priority:** second measured target. At 1,000 cells, projecting eight compatible
pending edits allocated 8.58 MB and took a median 18.393 ms, versus 0.691 ms for no
pending edits. This is component work, not a measured full frame duration.

**Files:** `collab/client/reconcile.go`, `executor.go`; narrow owned editor adapter
spans in `internal/app/ui/cpwsarea/wsmap/pmap/editor/collaboration.go`.

- [ ] Attribute cloning, indexing, validation and UI reprojection separately.
- [ ] Prototype cached immutable visible state or change-coordinate projection
  only after proving invalidation on acceptance, rejection, attachment changes,
  reconnect and conflict rebuilding. Keep submitted operation headers immutable.
- [ ] Test two pending edits where one conflicts, dependent pending edits, a
  rejection during a newer open gesture, queued completions after detach/close,
  delayed acknowledgements and undo/redo. Preserve inspectable failed intent.
- [ ] Benchmark 0/1/8/32 pending edits on compatible and conflicting workloads,
  then measure network-to-visible and input-to-visible distributions in the
  actual desktop. A faster projection that hides lost work fails acceptance.

## 4. Avoid replaying all retained history for each durable append

**Priority:** allocation evidence and a strong source hypothesis; elapsed-time
benefit is not established. At 100 cells, compacted history of 100 operations
raised measured append allocation from about 0.40 MB / 2,049 allocations to
1.56 MB / 14,400 allocations. Short SQLite timing ranges overlap substantially.

**Files:** `collab/store/sqlite/store.go`, `recovery.go`; PostgreSQL equivalents;
`collab/engine/recovery.go`; shared conformance fixtures.

- [ ] Profile SQL read/decode, prefix validation, suffix replay, transaction wait,
  hash construction, write and fsync separately for both databases.
- [ ] Investigate a versioned in-memory validated recovery cache with strict
  invalidation, or a private persisted recovery checkpoint retaining required
  hashes/inverse targets. No cache may assume exclusive ownership across store
  instances or trust a partially committed append.
- [ ] Preserve historical valid bases, actor-scoped inverses, already-inverted
  status, duplicate identity and unknown content. Do not truncate history or
  weaken recovery verification as a performance change without a retention and
  compatibility design. A schema/migration proposal needs independent review.
- [ ] Re-run concurrent store, cancellation, connection termination, committed-
  then-error, snapshot race, corruption, restart and logical backup/restore gates.
- [ ] Measure at fixed 0/100/1,000/10,000 history lengths using repeated longer
  trials on recorded storage, including full durability. Compare recovery and
  append costs; moving unbounded work from one to the other is not sufficient.

## 5. Investigate the remaining desktop and integration hypotheses

These have source evidence but no sampled user-visible bottleneck yet:

| Investigation | Concrete targets | Required decision evidence |
| --- | --- | --- |
| Full refresh for small edits | Editor `refreshCollaborationView`, `dmmsnap.Sync`, zones, prefab persistence and bucket updates | Stage timings and input-to-visible impact; incremental invalidations must preserve active gestures and level changes. |
| Export canonicalization | `collab/mapadapter/export.go`, DMM/TGM writer | 10,000-cell export measured 23.760 ms median. Measure actual staged Save, dictionary diversity and unknown content before changing the writer. |
| Idle frames and resize upload | `window/process.go`, canvas texture creation, brush stream upload | Idle/minimized CPU, frame pacing, draw counts, resize allocation and GPU timing. Respect animation and ImGui/input requirements. |
| Parser/FFI and icon lifetime | Go/Rust parser boundary, `dmenv.New`, icon cache/free and texture queue | Fresh/warm large environments, native copy/JSON attribution and repeated reload resource counts. No new FFI contract without measurements. |
| Search/tree/large selection | `cpsearch`, `cpenvironment`, `cpprefabs`, map tools and chunk layer rebuilds | Query and selection size sweeps; account for existing clipping, caching and culling. |
| Auth/telemetry/update/integration | OIDC registry, OTLP, bounded MCP adapter, staged artifacts, `internal/req` and updater | Latency and peak-memory distributions under local fixtures and approved external endpoints; no real executable replacement or production traffic for benchmarking. |

## Final comparison and qualification

- [ ] Keep control/candidate binary and fixture hashes, settings, raw samples and
  cold/warm labels. Randomize/interleave comparison order and investigate the
  large laptop variation observed in the initial baseline.
- [ ] Run the maintained `task verify`, affected race and real database gates,
  plus shipped-entry-point smoke and full desktop/service scenarios.
- [ ] Publish both positive and negative results, measured limits, and remaining
  unrun domains. Obtain named-human desktop acceptance and separate hosted CI /
  deployment evidence. Leave Git operations and protected infrastructure under
  their existing explicit-authorization rules.
