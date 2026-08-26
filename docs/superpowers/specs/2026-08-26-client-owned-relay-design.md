# AphelionDMM Client-Owned Collaboration Relay Design

**Status:** Approved implementation baseline  
**Date:** 2026-08-26  
**Supersedes:** The hosted-server authority and PostgreSQL requirements in `2026-08-24-multiplayer-design.md`  
**Preserves:** Deterministic operations, revision ordering, hash validation, actor-safe undo, map fidelity, and local single-user behavior

## Summary

AphelionDMM will move durable collaboration state and map authority from the public server to the desktop clients. The session owner's AphelionDMM process is the sole ordering authority while a session is active. Every participant stores the accepted snapshot, operation log, reconnect metadata, and their own pending submissions in a local SQLite database.

The public service becomes a lightweight WebSocket relay. It tracks only bounded, ephemeral connection metadata: rooms, connected client identities, roles, capability hashes, rate-limit counters, and last activity. It never receives plaintext map data and has no PostgreSQL, OIDC, snapshot, migration, backup, or document-recovery requirement.

Shared editing pauses whenever the owner is offline. Participants keep their last accepted local copy and pending work, but no client elects itself authority and no independent edit stream is accepted. When the owner returns, peers reconnect and reconcile from their local revision and hash.

The official client defaults to `https://mapping.a13.info`. Users can configure another compatible relay. Self-hosters receive one relay configuration file, a native Windows service package with a supervised Cloudflare connector, and one setup/operations entry point. This deployment detail is superseded by `2026-08-27-windows-relay-service-design.md` where this older design mentions containers.

## Goals

- Make each AphelionDMM client responsible for its own collaboration data.
- Keep the public service stateless with respect to maps and durable edits.
- Remove PostgreSQL and OIDC from the default online collaboration deployment.
- Preserve deterministic owner ordering, conflict behavior, revision hashes, and safe undo.
- Encrypt map snapshots, operations, and presence end to end between session participants.
- Recover from relay restarts without server backups or database restoration.
- Make the official public relay usable without a Cloudflare account or Cloudflare Access enrollment.
- Make the relay endpoint configurable and the server easy for others to reproduce.

## Non-goals

- Offline multi-master editing.
- Automatic owner election or consensus.
- Continuing to accept shared mutations while the owner is offline.
- Durable map, membership, invitation, or identity storage on the relay.
- A general-purpose peer-to-peer or CRDT system.
- Browser editing, source-code collaboration, or remote command execution.
- Content Tools integration. That work remains explicitly out of scope.

## Research basis

The existing `github.com/coder/websocket` dependency remains appropriate for the relay and desktop transport: it supplies context-aware WebSocket APIs, concurrent writes, close handshakes, and ping support without adding another transport stack. Cloudflare supports proxied WebSockets and public hostname routes to ordinary HTTP origins, so end users do not need `cloudflared` or Cloudflare Access on their machines. A Cloudflare Tunnel can keep the relay origin private while publishing the public hostname.

SQLite remains the local persistence format. It is already a project dependency and supports transactional local state without an external service. The database is an application-owned cache and journal, not the DMM source of record; saving a map continues to use the existing staged, validated, atomic map export path.

The local store uses one writer connection and SQLite WAL mode. The linked SQLite library must be version 3.51.3 or newer, or a documented patched backport, because SQLite documents a WAL-reset corruption bug in earlier releases. The current `modernc.org/sqlite` v1.57.0 module embeds SQLite 3.53.3 and satisfies this floor. The application verifies the runtime SQLite version when opening the collaboration store.

WebRTC and libp2p were rejected for the first online release. They can encrypt direct peer traffic and perform NAT traversal, but they introduce signaling, ICE/STUN/TURN or relay coordination, address discovery, and substantially more operational surface. A general-purpose CRDT was also rejected because DMM tile stacks, resize, environment replacement, unknown values, and safe inverse operations still require application-specific rules.

Primary references:

- [Coder WebSocket](https://github.com/coder/websocket)
- [Cloudflare proxied WebSockets](https://developers.cloudflare.com/network/websockets/)
- [Cloudflare Tunnel public-hostname routing](https://developers.cloudflare.com/tunnel/routing/)
- [SQLite write-ahead logging](https://www.sqlite.org/wal.html)
- [SQLite as an application file format](https://www.sqlite.org/appfileformat.html)
- [Pion WebRTC](https://github.com/pion/webrtc)
- [libp2p networking and NAT traversal](https://docs.libp2p.io/)
- [Automerge local-first documents](https://automerge.org/docs/hello/)

## Authority model

### Owner authority

The session owner runs the existing deterministic document engine locally. Only that process may:

- validate submitted operations against the accepted document;
- assign monotonically increasing revisions;
- decide conflicts;
- approve inverse operations;
- create authoritative snapshots;
- change roles or transfer ownership;
- sign authoritative collaboration messages.

The owner writes an accepted operation to local SQLite before signing and broadcasting its acceptance. A relay disconnection after that write cannot roll the owner back. Editors apply only owner-signed accepted operations.

### Participant replicas

Every participant persists:

- the current accepted snapshot and its environment/map hashes;
- accepted operations after that snapshot;
- acknowledged revision and owner signature;
- local actor identity and session profile;
- pending submissions and conflict drafts;
- reconnect and relay endpoint metadata.

An editor writes an accepted operation before acknowledging it to the owner. If the application exits during reconciliation, restart resumes from the last committed local revision.

### Owner absence

The relay knows which authenticated connection owns a room. When no valid owner connection is present, the room is `paused`:

- editor operation routing is rejected with `owner_offline`;
- viewer and editor clients retain their local accepted state;
- new peers may authenticate but cannot synchronize map content until the owner returns;
- presence may remain connected but must visibly report the paused state;
- no automatic election, merge, or split-brain recovery occurs.

The first implementation supports explicit owner transfer only while the current owner is online. Offline recovery of an abandoned session is a later, separately designed feature.

## Components

```text
AphelionDMM desktop
  collaboration UI and operation builder
    -> owner authority or participant replica
      -> deterministic operation engine
      -> local SQLite collaboration store
      -> encrypted, signed relay protocol
        -> configurable WebSocket relay

Public relay
  HTTP health/version endpoints
  WebSocket connection admission
  in-memory room and connection registry
  bounded routing queues and rate limits
  no map engine, snapshot store, OIDC, or database
```

### Desktop owner authority

This component adapts the existing `engine.Document` behavior to a local authoritative session. It owns revision assignment, validation, replay, snapshot generation, role changes, and owner signatures. It exposes a narrow message handler to the relay client rather than an HTTP service.

### Desktop participant replica

This component owns acknowledged state, speculative state, durable pending submissions, reconciliation, and local exports. It verifies the owner's signature before applying an acceptance or snapshot. The existing client state machine gains an explicit `paused_owner_offline` state.

### Local collaboration store

The store uses `modernc.org/sqlite` and a versioned schema under the user's application-data directory. One database may hold multiple sessions, but every row is scoped by session ID. SQLite transactions make snapshot replacement, operation append, acknowledgement, and pending-submission changes crash consistent.

The database does not replace `.dmm` or `.tgm` files. It supports collaboration recovery and may be safely rebuilt by rejoining an active owner, subject to session access.

### Relay

A new `cmd/apheliondmm-relay` process owns only:

- room creation and proof-of-owner reconnection;
- capability verification and connection admission;
- one active owner connection per room;
- role-aware routing between owner, editors, and viewers;
- bounded presence fanout;
- connection, room, byte, message, and rate limits;
- liveness, readiness, version, and aggregate metrics.

All room state is in memory and expires after the configured idle TTL. The relay does not persist invitations, identities, messages, snapshots, operations, map hashes, or display names.

## Identity, invitations, and encryption

### Client identity

Each installation creates an Ed25519 identity key. Public keys identify actors within invitations and signed messages. Private key material is never sent to the relay or other clients and must be stored using the operating system's user-protected credential facility where supported. A permission-restricted file is the explicit fallback on platforms without an implemented credential provider; the SQLite database never stores an unprotected private key.

Display names are client-owned profile data. An owner can change their display name like any other participant. Profile updates are signed, bounded, and ephemeral to the relay; peers persist the most recently accepted profile locally.

### Room creation

The owner generates:

- a random room ID;
- an Ed25519 owner signing key or designated session signing key;
- a random 256-bit group encryption key;
- a random relay owner capability;
- the initial owner-signed room manifest.

The relay receives the room ID, owner public key, owner capability hash, expiry, and limits. It never receives the group encryption key or plaintext room manifest.

### Invitations

An invitation is an owner-signed, bounded capability containing:

- protocol version;
- relay endpoint;
- room ID;
- invited role;
- one-use admission capability that binds to the first connecting actor key;
- issue and expiry times;
- group encryption key;
- owner public key and manifest fingerprint.

The client encodes this object into an `apheliondmm://join` URI. Platforms without protocol-handler registration support copy and paste of the same value. The relay sees the room ID and admission capability but not the encryption key. The capability is consumed by the first actor key that uses it; the owner persists that binding and restores it when recreating a room after relay restart. Reusable links are not part of protocol v2.

Revocation affects future connections by changing the relay admission set and issuing a new owner-signed manifest. Removing a currently connected participant closes that route. Full cryptographic exclusion requires group-key rotation and is performed when a participant is removed.

### Message protection

Payloads use XChaCha20-Poly1305 with a random nonce and bounded binary envelope. Room ID, protocol version, message class, sender public key, message ID, and sequence number are authenticated associated data. Durable submissions are signed by the submitting actor. Authoritative acceptances, rejections, snapshots, role changes, and session notices are signed by the owner.

The relay validates envelope size, message class, room, role, capability, sequence bounds, and rate limits without decrypting payloads. Clients reject duplicate message IDs, invalid signatures, stale room manifests, unexpected sender roles, and nonce reuse.

TLS remains mandatory for non-loopback endpoints even though application payloads are encrypted. It protects endpoint metadata, admission capabilities, and transport integrity.

## Protocol version 2

Protocol v2 is a separate compatibility boundary. A v1 hosted server and v2 relay must never be mistaken for one another.

### Relay-visible messages

- `room_create`: register an ephemeral room and owner proof.
- `admission_replace`: owner-signed replacement of the room's hashed, expiring one-use admission capabilities and restored actor bindings.
- `owner_transfer`: dual-signed replacement of the connected owner actor and owner signing key while both old and new owners are online.
- `connect`: authenticate a room capability and actor key.
- `route_owner`: editor-to-owner opaque payload.
- `route_actor`: owner-to-one-actor opaque payload.
- `route_room`: owner-to-room opaque payload.
- `presence_route`: bounded lossy opaque payload.
- `heartbeat`: maintain connection and room liveness.
- `disconnect`: close a connection with a stable reason.

### Encrypted application messages

- `sync_hello`: actor, role, acknowledged revision, map hash, environment hash, and pending-operation IDs.
- `sync_replay`: ordered owner-signed accepted operations after a known revision.
- `sync_snapshot`: owner-signed snapshot and revision/hash chain head.
- `sync_complete`: verified caught-up state.
- `operation_submit`: signed deterministic operation with base revision/hash.
- `operation_accepted`: owner-signed accepted operation and next revision/hash.
- `revision_acknowledged`: participant-signed confirmation sent only after its accepted revision is committed locally.
- `operation_rejected`: owner-signed stable conflict or validation result.
- `inverse_request`: signed actor request referencing an accepted operation.
- `profile_update`: signed display-name change.
- `presence_update`: lossy cursor, selection, viewport, and tool preview.
- `role_manifest`: owner-signed current membership and role state.
- `ownership_offer`, `ownership_accept`, and `ownership_transferred`: the dual-signed online owner-transfer handshake and resulting manifest.
- `session_notice`: owner-signed pause, resume, removal, key rotation, or shutdown notice.

Every durable application message participates in an owner-signed revision/hash chain. Presence does not.

## Synchronization and failure handling

### Initial join

1. The participant validates the invitation signature and endpoint policy locally.
2. The relay validates the admission capability and routes `sync_hello` to the owner.
3. The owner checks the actor and role against the current manifest.
4. The owner sends either a replay from the participant's known revision or a full encrypted snapshot.
5. The participant verifies signatures, revision continuity, environment hash, and final map hash inside one SQLite transaction.
6. The participant reports `sync_complete` and enters caught-up or read-only state.

### Reconnect

Clients reconnect with exponential backoff and jitter. A relay restart loses the room registry. The owner automatically recreates the same room by signing a fresh relay registration; other clients retry only after that succeeds. No server-side recovery step exists.

If an editor's acknowledged hash matches an owner-retained revision, the owner sends only later operations. Otherwise the owner sends a snapshot. Pending editor submissions are never silently resent: after synchronization, each is revalidated against current state and shown as ready, conflicting, or obsolete before submission.

### Desynchronization

Any signature failure, revision gap, unexpected map hash, or invalid operation result moves the participant to `desynchronized` and blocks edits. The client requests one bounded replay. If the replay does not establish the expected hash, it requests a full snapshot. If snapshot verification fails, the connection closes with a diagnostic code and preserves the local database for reporting. There is no infinite replay loop.

### Owner crash

Because the owner commits before acknowledgement, its local database is canonical. On restart it reconstructs the document, verifies its revision/hash chain, recreates the relay room, and resumes. If the owner database is corrupt or missing, the session does not automatically choose an editor copy. A guided owner recovery/import workflow is deferred until the basic relay migration is stable.

### Relay overload

Durable messages use bounded per-connection queues. A slow client is disconnected with `slow_consumer`; messages are not silently dropped. Presence uses a depth-one coalescing queue and may be dropped. Room and global limits fail closed with stable retryable errors.

## Configuration and deployment

### Client configuration

The shipped default relay is:

```yaml
collaboration:
  relay_url: https://mapping.a13.info
```

The collaboration settings UI allows a user to replace this URL. Non-loopback plain HTTP is rejected unless an explicit development-only command-line flag is present. Invitations may name another endpoint, but the client displays it and requires confirmation when it differs from the configured relay.

### Relay configuration

The relay reads one strict YAML file. Unknown keys are errors. Durations use Go duration syntax and sizes are bytes.

```yaml
version: 1
public_origin: https://mapping.a13.info
bind_address: 127.0.0.1:8080
trusted_proxy_cidrs:
  - 127.0.0.1/32
room_idle_ttl: 30m
limits:
  max_rooms: 1000
  max_connections: 4000
  max_connections_per_room: 32
  max_message_bytes: 1048576
  max_room_bytes_per_second: 8388608
  connect_burst: 30
  connect_window: 1m
  message_burst: 240
  message_window: 1s
observability:
  log_level: info
  metrics_bind_address: 127.0.0.1:9090
```

No database, OIDC, map directory, backup directory, or migration setting exists. Secrets are not placed in this file. A Cloudflare tunnel token remains a separate ACL-protected secret file because embedding it in a shareable configuration file would make safe replication harder.

### Automated operator path

The supported deployment is one native Windows service containing the relay runtime and supervising a dedicated `cloudflared` child. A single PowerShell entry point provides `setup`, `package`, `validate`, `install`, `start`, `status`, `logs`, `update`, `stop`, and `uninstall` actions. It validates package hashes, the pinned connector, file ACL expectations, the tunnel token, and the one Cloudflare public-hostname mapping needed by the operator.

The official deployment publishes `mapping.a13.info` as a public hostname with no Cloudflare Access policy. The origin binds only to loopback. Self-hosters may use the supervised Cloudflare Tunnel connector or another WebSocket-capable host-local reverse proxy.

### Scaling boundary

The initial relay runs as one replica because room membership is in memory. Its maximum useful scale is governed by connections and bandwidth, not map storage. Horizontal scaling, if required later, will use deterministic room-to-replica routing or a managed coordination layer; it will not reintroduce server-authoritative map storage.

## Migration plan boundary

The migration keeps v1 operational until v2 passes human online testing:

1. Add the local client store and owner/replica abstractions behind tests.
2. Add protocol-v2 crypto and compatibility fixtures.
3. Add the stateless relay and its native Windows service entry point.
4. Connect desktop owner and participant flows to the relay.
5. Add replay, snapshot, restart, pause, desync, and owner-name tests.
6. Add the single-file deployment and public-host operations path.
7. Run local two-instance, relay-restart, adverse-network, and public online pilots.
8. Make v2 the default hosted path only after acceptance.
9. Move the PostgreSQL/OIDC v1 deployment into a clearly labeled legacy location after the cutover; remove it only with separate approval.

Existing deterministic operation/model/engine code is reused. Existing PostgreSQL stores, hosted registry, OIDC flow, and server-side document recovery are not dependencies of protocol v2.

## Verification and acceptance

### Automated gates

- Local-store migration, transaction, corruption, and crash-recovery tests.
- Cross-platform protocol fixtures for encrypted envelopes, signatures, hashes, and rejection codes.
- Owner/replica conformance tests using the same deterministic engine fixtures.
- Relay tests proving it cannot parse map payloads and retains no room after expiry/restart.
- Duplicate, replay, role escalation, expired capability, key rotation, oversized message, and slow-consumer tests.
- Owner-offline pause and online owner-transfer tests.
- Desync escalation tests: replay once, snapshot once, then stable failure.
- Race tests for owner ordering, relay routing, reconnect, shutdown, and local-store access.
- Package and Windows-service tests proving startup requires only the relay configuration and optional tunnel secret.

### Human gates

- Two local clients edit, undo, rename profiles, disconnect, reconnect, and save identical maps.
- Relay restarts during idle, presence, and active editing without losing accepted state.
- Owner exits and editors visibly pause without accepting mutations; owner restart resumes cleanly.
- Public clients on separate networks join through `mapping.a13.info` without Cloudflare enrollment.
- A self-hoster follows only the documented setup path and changes the client relay URL successfully.
- Logs and diagnostics contain no map payload, invitation secret, encryption key, private key, or personal filesystem path.

## Documentation changes

Implementation updates must:

- mark the 2026-08-24 server-authoritative design as superseded for online protocol v2 while preserving its historical record;
- revise `docs/agent/multiplayer-invariants.md` from server authority to owner-client authority;
- publish the v2 OpenAPI/AsyncAPI or equivalent relay/application contracts;
- replace the PostgreSQL deployment handoff with the single-file relay guide;
- keep the v1 production guide visibly labeled as legacy until removal is approved;
- update the human testing guide with owner-offline, relay-restart, desync, profile-name, and privacy reporting checks.

## Deferred decisions

These require separate design and are not implementation placeholders:

- recovery when the owner permanently loses the local collaboration database;
- automatic owner election or offline owner transfer;
- continuing shared edits during owner absence;
- relay federation or horizontal room migration;
- reusable public invitations;
- CRDT or peer-to-peer transport.

## Decision record

- Durable map authority belongs to the session owner's client.
- Every client persists its own accepted collaboration state locally.
- The relay is ephemeral and database-free.
- Shared editing pauses while the owner is offline.
- Protocol v2 payloads are end-to-end encrypted and authoritative messages are owner-signed.
- `mapping.a13.info` is the default public relay, but clients and self-hosters can replace it.
- The first deployment is a single relay replica with one strict YAML file and a separate tunnel secret.
- Content Tools integration is excluded from this migration.
