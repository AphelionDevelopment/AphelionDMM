# ADMM audit remediation and qualification — September 5–6, 2026

## Status

Implemented repairs for the nine primary findings in the
[fresh code audit](2026-09-05-code-audit.md), leaving all changes uncommitted.
The local qualification gates below passed. The broader workplan is **not fully
qualified**: supplemental edge-case coverage, human desktop acceptance, real
Meridian/Content Tools integration and full runtime performance coverage remain
open. Prior reports were used as navigation, not as passing evidence.

Baseline HEAD remains `18429fc79bf69bb4720cfc92c13d028313438024`. This record refers
to the current dirty implementation, not to a commit containing the fixes. No
checkout/reset/merge/commit/push, worktree creation or delegation occurred.
Taskfiles, CI, deployment, signing, release and bootstrap files were not edited.
Public protocol/schema versions and database schema files were not changed.

## Repairs and direct evidence

| Finding | Implemented boundary | Reproduction and verification |
| --- | --- | --- |
| F1: recovery loses historical validation | `engine.RecoveryState` and `SessionStore.LoadRecovery` retain snapshot/head hashes, accepted operations, historical bases and inverse relationships. Memory, SQLite and PostgreSQL supply that context; append/snapshot/restart restore it before validating work. | SQLite undo, valid stale-base and acknowledged-tail probes failed before the repair and pass now. `TestAuditRecoveryRetainsUndoAndStaleBases` and shared store conformance cover repeated compaction/recovery, inverse safety and subsequent appends. Real PostgreSQL conformance also passed. |
| F2: persisted metadata ignored | Consistent SQL reads validate row identity/revision/hash against decoded data and the revision ledger; recovery verifies snapshot hash, contiguous history, replay hashes and durable head before serving the owner. | Original changed-snapshot/hash probe reproduced acceptance of corruption. `TestAuditSQLiteRejectsCorruptRecoveryRecords` now rejects changed snapshot metadata, operation identity/revision/hash and missing history/ledger cases. No corrupt database is rewritten to conceal failure. |
| F3: publication can reorder | Document owner publishes accepted nonduplicates after durable append and before responding. Each WebSocket writer enforces contiguous delivery and drains prior accepted events before sending a dependent rejection or duplicate response. Slow queues remain bounded. | Concurrent hub probe and deterministic owner-before-response probe went red then green. `TestAuditRejectionFollowsItsAuthorityOnWebSocket` forces a competing commit while the socket handles a delayed submission and verifies acceptance arrives before its rejection. Server/race gates passed. |
| F4: conflicting submitted drafts lost | Authoritative acceptance progresses even when a speculative operation no longer applies. Projection omits incompatible display effects while preserving the immutable submitted draft until the server resolves it. | `TestAuditConcurrentSameTileRetainsConflictDraft` reproduced lost authority/pending/conflict state, then passed. `TestAuditProjectionRetainsSubmittedBase` verifies remote progress does not rewrite a request already submitted. |
| F5: Save serializes speculative display | `Editor.SaveSnapshot` refuses open gestures and unresolved dispatch/acknowledgement work. `WsMap.Save` converts the captured authority into an isolated map for the existing staged writer. Failed saves retain dirty state; close callbacks honor false. Close dirty checks include acknowledged remote state; tab labels use cheap versions rather than full-map hashing every frame. | A real `PaneMap` with a hidden OpenGL context initially accepted a pending save. `TestSaveAcknowledgementBoundaries` now refuses it without changing the file, writes acknowledged direction 4 despite display direction 8, proves the edited prefab remains, balances only on success and preserves file/dirty state when backup loading fails. Save-before-close failure and authoritative-dirty checks passed. Native modal error presentation is queued and deliberately not clicked by this automated fixture. |
| F6: old callbacks erase gestures/reattach executors | Attachment generation fences asynchronous completion and history callbacks. Acknowledged-state updates retain a newer open gesture. Old callbacks cannot install their executor, and pane disposal fences completions before releasing resources. | `TestAuditAcknowledgementPreservesActiveGesture` went red then green and commits the second gesture through revision 2. `TestAuditOldAttachmentCompletionCannotRestoreExecutor` reproduces and verifies obsolete attachment isolation. |
| F7: normalized retries rejected | Server compares cloned operation intent independently of tile order and empty-collection representation, retaining all identity, base and content checks. | `TestAuditDuplicateUnsortedOperation` failed before the repair and now returns the accepted record without another revision. Engine's existing internal duplicate contract was not broadened or rewritten. |
| F8: older exact duplicate suspends client | Client retains the accepted event's own authoritative hash and validates duplicates against that record, rather than comparing an older event with the current head hash. | `TestAuditOlderExactAcceptedDuplicate` failed at head revision 2 before the repair and now preserves progress. General duplicate, transport, reconnect and client conformance gates passed. |
| F9: unknown prefab inspection panics | Missing type metadata is explicit; unknown prefabs expose preserved variables read-only with a neutral explanation. Default/filter/group/flags paths avoid nil metadata and do not invent defaults or silently edit unknown data. | Unknown-prefab setup reproduced a nil dereference. `TestAuditVariableEditorAcceptsUnknownPrefab` now covers explicit-value inspection, modified filtering, grouping and rejected direct mutation. Variable-editor, mapadapter, parser and editor suites passed. |

New implementation is owned under `internal/aphelion/`, with narrow marked spans
in inherited UI files. New source regressions live beside the exercised code.
The original audit overlays remain as historical reproduction inputs. Ownership
comments were added to the final new UI imports after runtime verification; this
did not change executable behavior or the benchmark harness.

## Qualification matrix

All raw logs below are under ignored `.artifacts/audit-2026-09-05/` unless noted.

| Gate | Result | Evidence and limits |
| --- | --- | --- |
| Focused repair regressions | Passed | Actual installed Go tests, including memory/SQLite compaction, corruption, ordering, duplicate, conflict, editor-generation, variable-editor and close-save behavior. Baseline failures are documented above and in the original audit. |
| `task verify` final run | Passed, exit 0 | `implementation-final-verify.log`: golangci-lint; contract/buildcheck/manifest gates; full Go suite; pinned Rust test/fmt/Clippy; release parser build; Windows desktop build. The Rust crate reported **0 unit tests**; its passing command is not parser behavior coverage. |
| Aphelion race suite | Passed, exit 0 | `implementation-race.log`, `go test -race ./internal/aphelion/... -count=1 -timeout 120s`. This run preceded the timestamp-only conformance comparison adjustment; final full Go and live PostgreSQL subsequently exercised that adjustment. No product concurrency code changed afterward. |
| Hidden-context real Save | Passed, exit 0 | `implementation-workspace-save.log`, explicit `APHELIONDMM_GL_TEST=1`; 0.82 s test time in the final run. A default suite skip does not supply this evidence. Hidden graphics construction and Save are not named-human UI acceptance. |
| Native PostgreSQL 18.6 | Passed, exit 0 | `implementation-postgres.log`; all live store/registry tests, concurrent writers, cancellation, migration contention, connection termination, SQLite equivalence, inverse/recovery conformance and logical backup/restore. Final suite 6.262 s. Each test used isolated schemas/databases in a disposable loopback cluster. All three audit-created clusters are stopped, with no remaining `postmaster.pid`. |
| Logical backup/restore | Passed in live suite | Restored exact revisions 1 and 2 and their expected hashes. Final observed restore durations 0.993 s and 1.089 s are functional-run timings, not a repeated backup performance benchmark. |
| Maintained CLI smoke | Passed, exit 0 | `implementation-smoke.log`, copied `implementation-smoke-report.json`: real fixture open/save/reparse, two-client loopback edits, equal client hashes at revisions 1–2, leave and shutdown. Input/output file SHA-256 both `7d044f7165ca68f32d823f463c6810876c91aa13934d4880fd96810469e9c9be`. This executable does not exercise the full desktop window. |
| Maintained doctor executable | Passed, exit 0 | `implementation-doctor.log`: Go 1.25.13; Rust 1.82.0; Task 3.53.1; golangci-lint 2.12.2; GCC 15.2.0. |
| Hosted Linux image build | Passed, exit 0 | `implementation-container-build.log`, checked-in `deploy/container/Dockerfile`, local Docker Desktop Linux engine. Test image `apheliondmm-audit:20260905`; image ID `sha256:4e696619f9e784a2f7a807648364cb81bc8fec2bd74bf9770dbb8a941c31c1c2`. No push/deployment. |
| Hosted image lifecycle | Passed, exit 0 | `implementation-container-lifecycle.log`, `TestHostedImageLifecycle`, 8.78 s. Nonroot/read-only/capability settings, OIDC fixture login, one-use invitations, snapshot persistence across restart, OTLP trace/metric export, database outage/readiness/recovery and logout. Disposable containers and network were removed. This is local container evidence, not hosted CI or a real identity-provider deployment. |
| Serial public-contract pilot | 5/5 passed | `.artifacts/performance-2026-09-05/pilot.log`; each run reached revision 250 and the same server hash with 25 sockets. Delivery p95 5.00–11.81 ms. Serial offering and later presence are explicit limitations; each client's applied hash is not verified by this harness. |
| Component performance | Measured, partial coverage | [Performance audit](2026-09-05-repo-performance-audit.md): five trials per workload, fixture/source/binary hashes, CPU/allocation profiles, and domain coverage matrix. No speedup or production budget claim. |
| Real Meridian-MCP / Rift / Content Tools | Unrun | Installed/staging test variables are not configured. Fixture/manifest package gates passed; no real game build, staged output acceptance or actual Content Tools contract consumer was exercised. No map was applied to another repository. |
| Hosted CI, production and human acceptance | Unrun | No remote workflow, deployment, production load, signing, release or named-human desktop session was requested/performed by this execution. |

The final desktop build before ownership-comment-only edits has SHA-256
`20943841e7885c3a18bee297b41fa8c894cd31ea5a24e062170a3f67e9d91f8c`.
Raw compiler output retains an upstream ImGui `memset` size warning; the build
succeeded, but that warning has not been independently diagnosed as harmless.

## Failures encountered and resolved

1. New regression-test lint found two unchecked deferred close results and one
   redundant embedded selector. They were corrected before the final gate.
2. The first disposable PostgreSQL helper used PowerShell `Start-Process -Wait`,
   which waited for the server child. Only that audit cluster was stopped; the
   helper now waits for the `pg_ctl` process itself and shuts the cluster down in
   `finally`. The failed launcher run is not source-failure evidence.
3. Live PostgreSQL exposed a pre-existing conformance assertion comparing
   `time.Time` structurally across UTC/local locations. The contract preserves
   instants, not Go location pointers. `sameCheckpointState` compares exact time
   instants and precision, completion nilness and every remaining field. A new
   test proves different instants and changed content still fail comparison.
   No storage timestamp was truncated and no failed test was skipped.
4. An initial benchmark invocation split unquoted dotted test flags in
   PowerShell; it executed no benchmark. Quoted flags were used for all recorded
   trials. The installed tool bundle lacked a `pprof` binary; the pinned Go source
   command successfully decoded the profiles.

## Remaining work and restart instructions

- The detailed original workplan deliberately contains broader combinations than
  the named regressions above. Complete rejection-during-a-new-gesture/dependent
  unsent-draft handling, queued render work after close, old undo/redo callback
  interleavings, altered older duplicates at multiple revisions, and unknown
  instance/pinned/environment-reload UI entry points before exhaustive acceptance.
  Current tests do not establish these combinations. In particular, a draft
  rejected locally before network submission is a separate boundary from F4's
  already-submitted draft retention.
- Run the [performance follow-up](../superpowers/plans/2026-09-06-admm-performance-followup.md):
  concurrent offered-load harness, per-client applied hashes, representative
  multi-Z/variable-heavy maps, native/graphics/desktop timings and a 30-minute
  lifecycle series. Full-history recovery remains deliberately expensive; no
  compaction/retention performance redesign is claimed here.
- Keep persisted-history assurance bounded: snapshot/head and replay hashes,
  metadata/continuity and compacted prefix relationships are checked. This is
  not cryptographic protection against coordinated modification of a database
  and all its historical hash records, nor proof that every possible corruption
  pattern is detected.
- Configure trusted `APHELION_MERIDIAN_MCP_REAL`, `APHELION_MERIDIAN_RIFT_ROOT` and
  `APHELION_MERIDIAN_STAGE_ROOT` for the installed-contract/staging-only tests.
  Review the exact staged target and use the maintained game acceptance entry
  point afterward. Actual Content Tools consumption is separate from schema tests.
- Restart/rebuild the desktop and collaboration service before testing the new
  code. Existing running processes do not gain these fixes. Persisted databases
  retain the same schema; no manual migration or generated-file edit is needed.
  Do not test repaired recovery against the only copy of a user's database.
- Perform named-human checks for overlapping drags, actor-scoped undo/redo,
  conflict rebuild, reconnect, delayed acknowledgements, Save/Save All and close
  with pending work. Local automated/container results do not replace this gate.

The [original workplan](../superpowers/plans/2026-09-05-admm-audit-workplan.md)
tracks these evidence boundaries rather than declaring every checkbox complete.
