# Windows relay-server deployment agent handoff

**Target:** Public protocol-v2 relay at `https://mapping.a13.info`

**Host:** Windows server that also runs Meridian-Rift

**Boundary:** One isolated native service, `AphelionDMMRelay`, supervising one dedicated Cloudflare connector process

## Objective

Install the stateless AphelionDMM relay without changing, restarting, or sharing storage with Meridian-Rift. Protocol v2 needs no PostgreSQL, OIDC provider, map storage, account database, backup job, container runtime, or public origin port. Clients own all durable collaboration data.

Deploy only an explicitly authorized immutable revision or ZIP artifact. Use an elevated PowerShell session for server changes. Never paste a tunnel token, invitation, group key, identity key, or client database into chat or logs.

## Required topology

```text
StrongDMM clients
    -> https://mapping.a13.info
    -> Cloudflare edge
    -> dedicated apheliondmm-mapping tunnel
    -> cloudflared child supervised by AphelionDMMRelay
    -> http://127.0.0.1:8080
    -> in-process stateless relay
```

- Do not reuse or modify the existing Meridian-Rift tunnel.
- Do not modify an existing `Cloudflared` Windows service.
- Do not create a Cloudflare Access application for `mapping.a13.info`.
- Do not expose port 8080 publicly or create an inbound firewall rule.
- A relay restart intentionally loses rooms; owners recreate them from client state.

## Read-only inventory

Before installation, record Windows version, free CPU/memory/storage, existing service names, listening ports, time synchronization, and outbound Cloudflare connectivity. Confirm that `AphelionDMMRelay` is unused and that `mapping.a13.info` has no competing route. Do not inspect existing service environment blocks or secret files. Stop if installation would require changing or stopping another service.

## Package and install

Build on a trusted workstation with:

```powershell
task package-relay-windows
```

Transfer `dst\relay-package\AphelionDMM-Relay-Windows-x64.zip` and verify its SHA-256 against the handoff record. Extract it on the server. No Go, Git, Docker, or source checkout is required there.

In an elevated PowerShell session in the extracted directory:

```powershell
& .\service.ps1 -Action Setup
notepad .\relay.yaml
& .\service.ps1 -Action Validate -TunnelTokenFile C:\secure\cloudflare_tunnel_token.txt
& .\service.ps1 -Action Install -TunnelTokenFile C:\secure\cloudflare_tunnel_token.txt
& .\service.ps1 -Action Status
```

The installer verifies manifest hashes plus the pinned connector's SHA-256 and Authenticode signature. It installs under `C:\Program Files\AphelionDMM Relay`, stores configuration and the ACL-restricted token under `C:\ProgramData\AphelionDMM\Relay`, runs as `LocalService`, configures delayed automatic start and SCM recovery, and starts the service.

Securely remove the temporary input token file after successful installation.

## Cloudflare route

Create a dedicated remotely managed tunnel, recommended name `apheliondmm-mapping`, with:

| Setting | Value |
| --- | --- |
| Hostname | `mapping.a13.info` |
| Service | `http://127.0.0.1:8080` |
| Cloudflare Access | Disabled |

The public endpoint must accept ordinary WebSocket clients without Cloudflare account membership. Reconfirm the pre-existing Meridian-Rift tunnel and service are unchanged.

## Certification

Verify locally and from a separate external network:

```powershell
Invoke-RestMethod http://127.0.0.1:8080/v1/health/live
Invoke-RestMethod http://127.0.0.1:8080/v1/health/ready
Invoke-RestMethod http://127.0.0.1:8080/v1/version
Invoke-RestMethod https://mapping.a13.info/v1/health/live
Invoke-RestMethod https://mapping.a13.info/v1/health/ready
Invoke-RestMethod https://mapping.a13.info/v1/version
```

Then run `docs/testing/client-owned-relay-human-test-guide.md` with two computers on separate networks. Restart only `AphelionDMMRelay`; verify owner-first room recovery, participant replay or snapshot, equal final revision and map hash, and zero plaintext or credential disclosure in Event Log output.

## Update, rollback, and removal

For a relay-only update, extract the approved new ZIP and run:

```powershell
& .\service.ps1 -Action Update -SkipCloudflare
```

The operation retains the installed dedicated connector/token and automatically restores prior relay files if readiness fails. A bad public route is rolled back by disabling only the `mapping.a13.info` route.

Uninstall with `-Action Uninstall`. Configuration and token data remain unless `-PurgeData` is explicitly supplied. The script refuses to remove a same-named service if its executable is not the expected AphelionDMM relay.

## Evidence and handback

Record the approved revision, ZIP SHA-256, package-manifest hashes, relay config SHA-256, service account/start/recovery state, connector version/hash/signature, tunnel name/UUID, DNS route, local and external health/version timestamps, WebSocket pilot result, restart-recovery result, privacy scan, and confirmation that Meridian-Rift and its tunnel were unchanged.

Report one of:

- **Ready for public two-network pilot**
- **Public route staged, certification incomplete**
- **Not deployed**, with the exact prerequisite or stop condition

The older `deploy/production` PostgreSQL/OIDC material applies only to server-authoritative protocol v1. Do not deploy it for protocol v2.
