# Client-owned relay agent handoff

**Status:** Native Windows service, packaging, installer, and CI gates implemented; public deployment pending

**Public endpoint:** `https://mapping.a13.info`

**Protocol:** AphelionDMM relay v2

## System contract

The relay is a disposable connection broker. Owners create rooms, publish admission digests, validate and order map operations, and persist authority locally. Participants persist acknowledged replicas and pending work locally. Application frames are signed and end-to-end encrypted. Relay restarts require owner-first reconnection, not server restore.

Never add a database, account system, map volume, user profile store, server-side invitation minting, or decryption key. Protocol-v1 hosted code is separate and legacy.

## Source roles

- `cmd/apheliondmm-relay`: console and Windows SCM entry point.
- `internal/aphelion/collab/relayruntime`: shared relay runtime.
- `internal/aphelion/collab/servicehost`: readiness gating and dedicated connector supervision.
- `internal/aphelion/collab/relay`: in-memory registry, routing, limits, health, version, and metrics.
- `internal/aphelion/collab/relayclient`: admission, signing, encryption, replay rejection, and client adapters.
- `internal/aphelion/collab/protocolv2`: public wire and invitation rules.
- `api/collaboration/relay-v2-asyncapi.yaml`: protocol contract.
- `deploy/relay/service.ps1`: package, validation, installation, operations, update, and uninstall.
- `docs/testing/client-owned-relay-human-test-guide.md`: public pilot procedure.

## Installed contract

- Windows service: `AphelionDMMRelay`
- Account: `NT AUTHORITY\LocalService`
- Programs: `C:\Program Files\AphelionDMM Relay`
- Configuration and connector token: `C:\ProgramData\AphelionDMM\Relay`
- Relay origin: `http://127.0.0.1:8080`
- Metrics: loopback only
- Logs: Windows Event Log source `AphelionDMMRelay`

The service runs the relay in-process and supervises its own `cloudflared.exe` child. It does not install or modify Cloudflare's fixed-name Windows service and must not reuse the Meridian-Rift tunnel.

## Configuration and secret

`relay.yaml` is the only application configuration. It holds the public origin, loopback bind addresses, trusted proxy CIDRs, limits, TTL, and log level. The tunnel token is a separate ACL-restricted file because it is secret and must not be placed in shareable configuration.

## Operator interface

```powershell
& .\service.ps1 -Action Setup
& .\service.ps1 -Action Package -Force
& .\service.ps1 -Action Validate -TunnelTokenFile C:\secure\token.txt
& .\service.ps1 -Action Install -TunnelTokenFile C:\secure\token.txt
& .\service.ps1 -Action Start
& .\service.ps1 -Action Status
& .\service.ps1 -Action Logs
& .\service.ps1 -Action Update -SkipCloudflare
& .\service.ps1 -Action Stop
& .\service.ps1 -Action Uninstall
```

There are deliberately no backup, restore, migration, database, or user-management actions.

## Public routing

Use a dedicated remotely managed tunnel and route `mapping.a13.info` to `http://127.0.0.1:8080`. Cloudflare Access must be disabled. The service needs outbound connectivity only; do not open port 8080 in the inbound firewall.

## Recovery and limits

After a relay restart, the owner recreates the room and admissions from local state, then participants reconnect and receive replay or snapshot from the owner. Pending local work remains pending. If the owner client is unavailable, editing remains paused.

The protocol validates dual-signed owner transfer, but complete desktop owner-transfer UX remains deferred. Same-profile dual-instance tests share an installation identity, so public acceptance must use separate computers or isolated user profiles.

## Protected boundary

The native Windows service deployment and CI set was explicitly approved on 2026-08-27. Future changes to `deploy/relay/*`, `.github/workflows/ci.yml`, or `Taskfile.yml` require a new exact-file/effect review.
