# Multiplayer invariants

## Protocol v2: client-owned relay sessions

1. Exactly one connected owner client orders and validates durable mutations.
2. Every accepted operation is durably appended by the owner before broadcast and receives one monotonically increasing revision.
3. Every participant persists its own acknowledged replica. The relay persists none of it.
4. Duplicate operation IDs are idempotent; explicit before-values prevent silent overwrites.
5. A participant never promotes speculative or pending work to acknowledged state without an owner acceptance.
6. Pending work survives locally but is not automatically resent after reconnect. The user must review it.
7. Snapshot or contiguous replay installation is transactional and must match the signed document and environment context.
8. Owner loss pauses durable editing. Relay loss does not change the last acknowledged client state.
9. A relay restart requires room/admission recreation by the owner, then participant resynchronization; it requires no database restore.
10. Ownership transfer is atomic only after dual signatures over the exact resulting manifest and relay acknowledgement while both actors are connected.

## Operations and conflicts

Durable operations contain protocol, document, actor and operation IDs, base revision/hash, environment hash, kind, explicit changes, and replacement preconditions. Validation is all-or-nothing. Non-overlapping edits may merge by owner order; conflicting stale values reject with bounded authoritative context. Undo and redo are new actor-scoped operations with ordinary preconditions, never history rewinds.

## Identity, profiles, and roles

Actors authenticate protocol messages with installation keys. Invitations carry signed owner/session/document/environment context plus a short-lived capability and group key. The relay stores only the capability digest and binding. Roles are owner, editor, and viewer. Viewer mutation is rejected locally and authoritatively. Profile sequences must increase; display names are bounded UTF-8 values and are not identity keys.

## Presence and relay state

Presence is ephemeral, lossy, bounded, and excluded from map hashes and durable logs. Relay room, connection, rate-limit, and admission state is disposable. Metrics expose aggregate connection/room counts only. Relay logs must not contain map content, paths, names, invitations, capabilities, group keys, or private keys.

## Map fidelity and persistence

Unknown DreamMaker types and variables survive parse, operation, snapshot, and save round trips. Stable IDs never derive from process-local counters. Canonical hashes use the versioned representation. Saves are staged, reparsed, hash-checked, flushed, and atomically replaced; failure preserves the prior file.

## Protocol-v1 legacy invariants

Protocol v1 remains server-authoritative and uses server persistence, resumption tokens, and hosted OIDC. Its invariants are preserved in the historical 2026-08-24 design and v1 tests. Never mix v1 server authority or PostgreSQL recovery assumptions into protocol-v2 code or documentation.
