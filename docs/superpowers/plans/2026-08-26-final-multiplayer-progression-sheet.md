# AphelionDMM multiplayer progression sheet

**Status date:** 2026-08-26  
**Primary online design:** Client-owned protocol v2 with a stateless public relay
**Excluded:** Content Tools implementation, public session discovery, and general release publication

## Bottom line

Protocol-v2 foundations, local persistence, owner authority, encrypted relay routing, desktop start/join/edit paths, explicit reconnect, real relay/load smoke paths, and the protected relay container/operator/CI bundle are implemented and locally verified in the working tree.

Automated work has reached the public deployment and human-test gate. The remaining first-pilot prerequisite is to deploy and externally verify `mapping.a13.info`. Desktop owner transfer is explicitly deferred from the first pilot; it remains required before release acceptance.

## Progression

| Area | Implemented and automated | Locally exercised | Publicly accepted | Remaining |
| --- | --- | --- | --- | --- |
| Deterministic operation engine and atomic map path | Yes | Yes | No | Public save/reopen and conflict/undo pilot |
| Protocol-v2 signed/encrypted envelope and invitations | Yes | Focused tests | No | Cross-build fixture and public invitation exercise |
| Client identity and protected secret references | Yes | Focused tests | No | Two-machine identity/restart exercise |
| Client-owned SQLite authority/replica durability | Yes | Focused crash/reopen tests | No | Desktop process interruption and pending-state observation |
| Owner authority, replay/snapshot, conflicts, inverse | Yes | Real in-process relay convergence | No | Adverse desync pilot |
| Stateless relay registry/router/service | Yes | Command, container, and relay-restart tests | No | Public Cloudflare route |
| Abuse/privacy controls | Yes | Focused, smoke, static, and image-scan assertions | No | Public log review |
| Desktop online start/join/edit/viewer/name paths | Yes | Windows build, automated app tests | No | Two-machine UI and profile propagation |
| Manual relay reconnect | Implemented | Package/app gates | No | Real desktop owner-first restart exercise |
| Automatic reconnect | No | No | No | Manual recovery accepted for first pilot; reconsider from pilot evidence |
| Online owner transfer | Core only | Authority/relay tests | No | Deferred from first pilot; complete desktop offer/accept/role transition before release |
| Load runner | 2/8/32 opaque routing | Repeated race gate | No | Durable bursts, slow consumer, invalid/expired/replay and desync scenarios |
| Relay deployment bundle | Yes | Full operator and image lifecycle | No | Install on dedicated host and publish route |
| Agent/operator/test documentation | Yes | Documentation/privacy gates | No | Add public-pilot evidence after testing |
| Meridian integration | Existing staged-map contracts retained | Not rerun in this branch | No | Post-relay map acceptance when requested |

## Verified in the current implementation session

- Focused protocol-v2, identity, authority, replica, store, relay-client, UI, and app tests passed.
- Relay and relay-client race tests passed.
- The shipped `apheliondmm-relay` command built, became healthy, and routed encrypted owner/editor traffic without logging the fixture display name.
- The client-owned load runner passed 2, 8, and 32 participants repeatedly under the race detector after repairing an admission-publication ordering race with bounded participant retry.
- `task build` produced `dst/StrongDMM.exe`; the current recorded SHA-256 is `E8DD878CFDDAB5CD400D22123825AC5340F28C414292497D7A178549459741D7`.

The dated Task 13 evidence is in `docs/verification/client-owned-relay-automated-2026-08-26.md`. These remain local automated results, not public acceptance.

## Remaining work before human testing

### Deployment prerequisite

- Install the reviewed relay bundle on the dedicated host from an authorized immutable revision.
- Create a dedicated `apheliondmm-mapping` tunnel without modifying the existing `bark` tunnel.
- Route `mapping.a13.info` to `http://relay:8080` without Cloudflare Access.
- Verify public health, readiness, version, WebSocket routing, and sanitized logs from an external network.

### Human-only after those gates

- Deploy `mapping.a13.info` through a dedicated public tunnel without Cloudflare Access.
- Use two computers on separate networks.
- Exercise names, editor/viewer roles, conflicts, inverse/redo, owner-offline pause, relay restart, explicit reconnect, desync recovery, save/reopen fidelity, endpoint confirmation, keyboard/narrow UI, and privacy. Owner transfer is excluded from this first pilot.
- Record both clients' revisions/hashes and sanitized evidence.

## Legacy protocol v1

The PostgreSQL/OIDC hosted stack remains available as protocol-v1 compatibility material. It is no longer the target architecture for the public protocol-v2 pilot. Do not delete or cut it over without a separate post-pilot approval.

## Decision table

| Target | Decision |
| --- | --- |
| Continue automated implementation | **Automated pre-pilot gates complete** |
| Begin public two-network pilot | **Ready after public route deployment and external health/WebSocket check** |
| Deploy PostgreSQL/OIDC for v2 | **Do not do this** |
| Publish `mapping.a13.info` without Cloudflare Access | **Next deployment action; Cloudflare inventory is conflict-free** |
| Claim multiplayer complete | **Not justified until public pilot passes** |

All changes remain uncommitted and unpushed unless separately authorized.
