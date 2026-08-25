# Multiplayer invariants

These rules are correctness requirements, not implementation preferences.

## Authoritative state

1. One server-side document owner serializes every durable mutation.
2. Each accepted operation receives exactly one monotonically increasing document revision.
3. An acknowledgement is sent only after the operation is durably recorded at the configured durability level.
4. Duplicate `(document_id, operation_id)` delivery returns the original result and never reapplies the operation.
5. Applying the accepted log to the same validated snapshot produces the same canonical map hash on every supported platform.

## Operations

Every durable operation contains:

- protocol version;
- document, actor, and operation identifiers;
- base revision;
- environment hash and the canonical map hash at the named base revision;
- explicit typed changes;
- preconditions for values being replaced;
- an operation kind with bounded payload rules.

The server validates the entire operation before applying any part. A stale base revision may still merge when its base hash is authentic and all explicit value preconditions remain valid. Partial acceptance is not permitted.

Operations describe domain changes, not UI gestures. A brush drag is converted to deterministic tile changes before submission. The accepted record contains the normalized change set used by the server.

## Conflict behavior

- Non-overlapping tile or property changes may be accepted in server order.
- A precondition mismatch rejects the conflicting change and returns current authoritative values.
- Map resize, environment replacement, import, and export-finalization are exclusive maintenance operations.
- Clients reconcile from accepted operations; they never declare local speculative state authoritative.
- A reconnect starts from an acknowledged revision and receives a snapshot or contiguous replay sufficient to reconstruct current state.

## Undo and redo

Undo is a new inverse operation authored by the requesting actor. It names the target operation and carries preconditions proving the target values are still safely reversible. It cannot erase history, rewind other actors, or move a shared history pointer. Redo is a new forward operation subject to the same validation.

## Presence

Cursor, selection, viewport, tool preview, typing, and user status are ephemeral. They are not written to the durable operation log, do not affect map hashes, may be dropped or coalesced, and expire after disconnect or timeout.

## Map fidelity and persistence

- Unknown DreamMaker types and variables survive parse, operation, snapshot, and save round trips.
- Stable collaboration identifiers are explicit and never derived from process-local counters.
- Canonical hashes use a specified byte representation and ordering.
- Snapshots include protocol, schema, environment, map, and last-revision metadata.
- A saved DMM/TGM is staged, reparsed, hash-checked, flushed, and atomically replaced.
- The previous target remains intact on failure.

## Compatibility

Protocol negotiation is explicit. Additive fields are optional only when their absence has a defined meaning. Semantic changes require a new protocol version and compatibility fixtures. Clients that cannot preserve a document's fidelity join read-only or are rejected.
