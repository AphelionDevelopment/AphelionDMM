# Game-server deployment agent handoff

**Target:** Public protocol-v2 relay at `https://mapping.a13.info`
**Host:** Dedicated server that also runs Meridian-Rift
**Boundary:** One isolated stateless relay container and, when used, one dedicated Cloudflare Tunnel sidecar

## Objective

Run the AphelionDMM relay without changing, restarting, or sharing storage with Meridian-Rift. Protocol v2 needs no PostgreSQL, OIDC provider, map storage, account database, backup job, or public origin port. Clients own all durable collaboration data.

Read `AGENTS.md`, the agent security/verification guides, the protocol-v2 design, and `client-owned-relay-agent-handoff.md` before acting. Deploy only an explicitly authorized immutable revision. Use PowerShell for Windows host work. Never paste a tunnel token, invitation, group key, identity key, or client database into chat or logs.

## Required topology

```text
StrongDMM clients
    -> https://mapping.a13.info
    -> Cloudflare edge
    -> dedicated apheliondmm-mapping tunnel
    -> http://relay:8080
    -> stateless apheliondmm-relay
```

- Do not reuse or modify the existing Meridian-Rift tunnel.
- Do not create a Cloudflare Access application for `mapping.a13.info`.
- Do not expose relay port 8080 publicly. Use the loopback overlay only for local certification.
- Do not mount Meridian-Rift files, maps, repositories, Docker socket, or persistent data into the relay.
- A relay restart intentionally loses rooms. Owners recreate them from local client state.

## Read-only inventory

Before installation, record Docker Engine/Compose versions, available CPU/memory/storage, existing container names/networks, listening ports, time synchronization, service-account ownership, and outbound Cloudflare connectivity. Do not inspect existing container environment blocks. Stop if the new Compose project would conflict with or require stopping an existing service.

Confirm that the deployment revision is clean and approved, and that `mapping.a13.info` has no competing DNS route. Record sanitized evidence under `docs/verification/`.

## Operator path

The reviewed relay bundle provides one entry point:

```powershell
& .\deploy\relay\operations.ps1 -Action Setup
& .\deploy\relay\operations.ps1 -Action Validate
& .\deploy\relay\operations.ps1 -Action Build
& .\deploy\relay\operations.ps1 -Action StartLoopback
& .\deploy\relay\operations.ps1 -Action Status
& .\deploy\relay\operations.ps1 -Action Stop
```

After loopback certification, install the dedicated tunnel token directly at `deploy/relay/secrets/cloudflare_tunnel_token.txt`, then run:

```powershell
& .\deploy\relay\operations.ps1 -Action StartCloudflare
& .\deploy\relay\operations.ps1 -Action Status
```

The protected files were explicitly confirmed and locally exercised on 2026-08-26. Deploy the reviewed bundle from an authorized immutable revision; do not reproduce or improvise it on the server.

## Cloudflare route

Create a dedicated remotely managed tunnel, recommended name `apheliondmm-mapping`, with one public hostname:

| Setting | Value |
| --- | --- |
| Hostname | `mapping.a13.info` |
| Service | `http://relay:8080` |
| Cloudflare Access | Disabled |

The public endpoint must accept ordinary client WebSockets without Cloudflare account membership. Reconfirm the pre-existing Meridian tunnel is unchanged.

## Certification

From loopback, then a separate external network, verify:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/v1/health/live
Invoke-RestMethod http://127.0.0.1:8080/v1/health/ready
Invoke-RestMethod http://127.0.0.1:8080/v1/version

Invoke-RestMethod https://mapping.a13.info/v1/health/live
Invoke-RestMethod https://mapping.a13.info/v1/health/ready
Invoke-RestMethod https://mapping.a13.info/v1/version
```

Then follow the client-owned relay human test guide with two computers on separate networks. Inspect relay logs and metrics for map text, paths, display names, invitations, capabilities, group keys, and private keys; expected disclosure is zero.

Restart only the relay. The owner must reconnect first and recreate the room, followed by participants. Confirm the final client revisions and map hashes match. No restore operation should exist or be required.

## Update and rollback

`operations.ps1 -Action Update` should build or pull the authorized image, replace only this Compose project's relay, wait for health, and retain the prior image tag for manual rollback. A bad public route is rolled back by disabling only the new `mapping.a13.info` route. A bad relay release is rolled back to the recorded prior image. Never delete client data: it is not on this server.

## Evidence and handback

Record the approved revision, image ID/digest, non-secret relay-config SHA-256, Docker versions, tunnel name/UUID, DNS route, health/version timestamps, external WebSocket result, relay-restart result, privacy scan, and confirmation that Meridian-Rift and its tunnel were unchanged.

Report one of:

- **Ready for public two-network pilot**
- **Public route staged, certification incomplete**
- **Not deployed**, with the exact prerequisite or stop condition

## Legacy protocol-v1 material

The older `deploy/production` PostgreSQL/OIDC stack and its backup/restore procedure apply only to server-authoritative protocol v1. Preserve them as legacy evidence, but do not deploy them for the protocol-v2 relay.
