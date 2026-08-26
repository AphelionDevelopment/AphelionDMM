# Architecture

## Current application

AphelionDMM is an Aphelion-owned StrongDMM downstream. The inherited Dear ImGui desktop, mutable map model, DMM/TGM serializer, and vendored Rust parser remain in place. Aphelion collaboration enters the editor through the narrow executor seam in `internal/app/ui/cpwsarea/wsmap/pmap/editor`.

## Protocol-v2 online architecture

```text
Owner desktop
  UI -> owner executor -> owner authority -> local SQLite
                         -> encrypted/signed operation broadcast
                                      |
                              stateless relay
                                      |
Participant desktop                    |
  UI -> replica executor -> local SQLite <- encrypted/signed replay/snapshot
```

- `internal/aphelion/collab/model` and `engine` own deterministic operations, conflicts, hashes, and inverse operations.
- `authority` owns protocol-v2 validation, ordering, membership, signed manifests, invitations, and the current document while the owner is online.
- `replica` owns participant snapshot/replay installation and durable pending-operation records.
- `store/sqlite` persists each client's own session, snapshot, accepted log, manifests, admissions, and pending work.
- `protocolv2` owns the signed binary envelope, encrypted application payloads, invitations, and control messages.
- `relay` routes opaque frames and keeps only short-lived connection, room, admission-digest, role, and rate-limit state in memory.
- `relayclient` adapts owner and participant state machines to the WebSocket relay.
- `cmd/apheliondmm-relay` is the public stateless relay executable.

The relay never parses application plaintext, decides map conflicts, stores a map, mints user identity, or restores a session. The owner recreates relay routing state after a relay restart. Participants synchronize from the owner and pause when it is unavailable.

## Identity and ownership

Each installation has an Ed25519 identity protected by the platform secret store. A session has a random room ID and group encryption key. The owner signs role manifests and short-lived, one-use invitations. An admitted participant binds its capability to its public key. Ownership transfer requires both the current owner and target editor to sign the exact resulting manifest before the relay changes routing authority.

## Desktop lifecycle

The default online endpoint is `https://mapping.a13.info`; users may configure another HTTPS endpoint. Loopback HTTP is permitted for local testing. Starting an online session creates authority and local persistence before attaching the executor. Joining verifies the invitation, environment, endpoint, and owner signature before installing replay or snapshot state. Relay transport failures disable mutation and expose reconnect rather than silently returning to local editing.

## Protocol-v1 legacy architecture

`internal/aphelion/collab/server`, the PostgreSQL/OIDC hosted service, and embedded local service are retained for compatibility and migration evidence. Protocol v1 is server-authoritative and uses different identity, persistence, invitation, and recovery rules. New online work targets protocol v2 unless the task explicitly names the legacy service.

## Integration boundary

Meridian-MCP and Meridian-Rift consume staged maps, hashes, and versioned contracts; they are not collaboration transports or relay authorities. Content Tools integration is deferred and is not part of the protocol-v2 relay rollout.

