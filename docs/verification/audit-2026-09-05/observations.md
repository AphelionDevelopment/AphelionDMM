# Audit probe observations

Date: 2026-09-05. Application source: `18429fc79bf69bb4720cfc92c13d028313438024`. Go: `1.25.13 windows/amd64`. These are selected failure observations from live tool output, not a transcript of every compiler line. Production source was unchanged. The probes assert required correct behavior and therefore exit with code 1 on the audited revision.

| Probe | Observed failure | Finding |
| --- | --- | --- |
| `TestAuditSQLiteUndoAcrossSnapshot` | `safe inverse rejected after snapshot` → `operation_not_found` at revision 1 | F1 |
| `TestAuditSQLiteStaleBaseAcrossSnapshot` | authentic nonoverlapping edit rejected → `unknown_base_revision`, base revision 0 not retained | F1 |
| `TestAuditSQLiteSnapshotCanMakeAcknowledgedLogUnrecoverable` | acknowledged revision 2 cannot recover after snapshot at 1 → replay rejects base revision 0 | F1 |
| `TestAuditSQLiteRecoveryChecksStoredHash` | `corrupt snapshot served: max_x=4, retained max_x=3; stored hash was not checked` | F2 |
| `TestAuditHubBroadcastsInRevisionOrder` | `round 0: broadcast revision=4, want=1 (arrival order)` | F3 |
| `TestAuditConcurrentSameTileRetainsConflictDraft` | valid remote acceptance fails while reapplying speculation; `acknowledged=0 pending=false conflicts=0` | F4 |
| `TestAuditAcknowledgementPreservesActiveGesture` | `acknowledgement of first edit erased active second gesture; its commit now returns without submitting` | F6 |
| `TestAuditDuplicateUnsortedOperation` | identical retry rejected as conflicting with stored revision 1 | F7 |
| `TestAuditOlderExactAcceptedDuplicate` | exact duplicate suspends executor → `accepted revision is 1, want 3` | F8 |
| `TestAuditVariableEditorAcceptsUnknownPrefab` | `opening preserved unknown prefab panicked: runtime error: invalid memory address or nil pointer dereference` | F9 |

The original core probe invocation ran the first five server cases and two client cases. The subsequently added hub probe ran separately with `-Filter '^TestAuditHub'`. The editor and unknown-prefab probes ran separately with `-Editor` and `-Unknown` using normal filesystem access. All four completed invocations exited 1 with the failures above. The current runner includes all of these cases through its corresponding switches.

An earlier attempt failed before executing probes because of access denied to the default Go compiler cache. The core probes were then compiled and executed with the repository-local ignored `.artifacts/audit-2026-09-05/go-cache`. That setup failure is excluded from the behavior observations above.

The hub probe is a bounded scheduler-sensitive concurrency test. The remaining cases arrange their relevant event/snapshot order explicitly. These probes do not establish a deployed service, interactive desktop, live PostgreSQL, or full native-build result.
