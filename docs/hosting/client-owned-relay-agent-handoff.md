# Client-owned relay agent handoff

**Status:** Container, operator, and CI bundle implemented and locally verified; public deployment pending  
**Public endpoint:** `https://mapping.a13.info`  
**Protocol:** AphelionDMM relay v2

## System contract

The relay is a disposable connection broker. Owners create rooms, publish admission digests, validate and order map operations, and persist authority locally. Participants persist acknowledged replicas and pending work locally. Application frames are signed and end-to-end encrypted. Relay restarts require owner-first reconnection, not server restore.

The relay must never gain a database, account system, map volume, user profile store, server-side invitation minting, or decryption key. Protocol-v1 hosted code remains separate and legacy.

## Source roles

- `cmd/apheliondmm-relay`: strict `-config` command, signals, bounded shutdown.
- `internal/aphelion/collab/relay`: in-memory registry, routing, WebSocket service, limits, health/version/metrics.
- `internal/aphelion/collab/relayclient`: client admission, signing/encryption, replay rejection, owner/participant adapters.
- `internal/aphelion/collab/protocolv2`: public wire and invitation rules.
- `api/collaboration/relay-v2-asyncapi.yaml`: protocol contract.
- `deploy/relay`: reviewed container, Compose overlays, strict configuration, and operator entry point.
- `docs/testing/client-owned-relay-human-test-guide.md`: public pilot procedure.

## Configuration schema

One non-secret YAML file configures version, public origin, bind address, trusted proxy CIDRs, room TTL, connection/message/byte limits, log level, and the private metrics bind address. The checked-in example is the schema authority. Secrets must not be added to this file.

For Cloudflare, the only secret is a dedicated tunnel token stored as `deploy/relay/secrets/cloudflare_tunnel_token.txt`. It is mounted only into the tunnel sidecar. Do not print it, put it in `.env`, or commit it.

## Expected operator interface

```powershell
& .\deploy\relay\operations.ps1 -Action Setup
& .\deploy\relay\operations.ps1 -Action Validate
& .\deploy\relay\operations.ps1 -Action Build
& .\deploy\relay\operations.ps1 -Action StartLoopback
& .\deploy\relay\operations.ps1 -Action StartCloudflare
& .\deploy\relay\operations.ps1 -Action Status
& .\deploy\relay\operations.ps1 -Action Logs
& .\deploy\relay\operations.ps1 -Action Update
& .\deploy\relay\operations.ps1 -Action Stop
```

There are deliberately no backup, restore, migration, database, or user-management actions.

## Public routing

Use a dedicated tunnel and route `mapping.a13.info` to `http://relay:8080`. Do not use Cloudflare Access. Do not reuse the Meridian-Rift tunnel. Trust Cloudflare client-address headers only from the Compose network CIDR containing the tunnel sidecar.

## Health and privacy

Public checks:

```powershell
Invoke-RestMethod https://mapping.a13.info/v1/health/live
Invoke-RestMethod https://mapping.a13.info/v1/health/ready
Invoke-RestMethod https://mapping.a13.info/v1/version
```

`/metrics` exposes only aggregate active connections and rooms. Relay logs must not contain client plaintext, display names, map paths/content, invitations, capabilities, group keys, or identity private keys.

## Recovery

After a relay restart:

1. Owner selects Retry Reconnect, recreating the room and admissions from local state.
2. Participants select Retry Reconnect.
3. Participants receive replay or snapshot from the owner.
4. Pending local work remains pending and is not silently resent.
5. Compare client revision and map hash.

If the owner client is unavailable, editing remains paused. Do not invent a server repair or restore procedure.

## Known pre-pilot limits

The protocol and relay validate dual-signed owner transfer, but the complete desktop transfer action is not yet wired. Automatic retry is not claimed; the current recovery control is explicit. Same-profile dual-instance tests share an installation identity, so public acceptance should use two computers or isolated user profiles.

## Protected-file boundary

The protocol-v2 relay deployment and CI file set was explicitly confirmed on 2026-08-26. Future changes to `deploy/relay/*`, `.github/workflows/ci.yml`, or `Taskfile.yml` still require a new exact-file/effect review under `AGENTS.md`.
