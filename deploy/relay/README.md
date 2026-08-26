# AphelionDMM relay Windows service

Protocol v2 uses one native Windows service named `AphelionDMMRelay`. The service runs the stateless relay in-process and supervises a dedicated `cloudflared.exe` child. Docker is not used. Existing services, including a fixed-name `Cloudflared` service or a Meridian-Rift tunnel, are not modified.

The relay stores no maps, operations, accounts, profiles, or database. Clients retain durable collaboration state and reconstruct rooms after a relay restart.

## Requirements

- 64-bit Windows Server
- An elevated PowerShell session for install, update, and uninstall
- An approved source revision or the packaged Windows ZIP
- A dedicated remotely managed Cloudflare Tunnel token for public service
- Outbound HTTPS connectivity; no inbound firewall rule is required

Configure the dedicated tunnel's public hostname in Cloudflare:

```text
mapping.a13.info -> http://127.0.0.1:8080
```

Do not enable Cloudflare Access. Ordinary clients must connect without membership in the operator's Cloudflare account.

## Build a distributable ZIP

From the repository root:

```powershell
task package-relay-windows
```

The result is `dst\relay-package\AphelionDMM-Relay-Windows-x64.zip`. A server does not need Go, Git, or the source tree when installing from this ZIP.

## First installation

Extract the ZIP, open an elevated PowerShell session in the extracted directory, and create the editable configuration:

```powershell
& .\service.ps1 -Action Setup
notepad .\relay.yaml
```

The checked-in example is the schema reference. `relay.yaml` is non-secret. Keep the tunnel token in a separate temporary file and never add it to YAML, terminal history, chat, or source control.

Validate and install:

```powershell
& .\service.ps1 -Action Validate -TunnelTokenFile C:\secure\cloudflare_tunnel_token.txt
& .\service.ps1 -Action Install -TunnelTokenFile C:\secure\cloudflare_tunnel_token.txt
```

The installer verifies packaged file hashes, the pinned `cloudflared` SHA-256, and its Authenticode signature before creating the service. It copies the token to an ACL-restricted data directory, so the temporary input file can then be securely removed.

Installed layout:

```text
C:\Program Files\AphelionDMM Relay\
C:\ProgramData\AphelionDMM\Relay\
```

The service runs as `NT AUTHORITY\LocalService`, starts automatically with delayed start, and has Service Control Manager recovery configured. Relay and metrics listeners bind to loopback only.

## Routine operation

Run these from the extracted package directory:

```powershell
& .\service.ps1 -Action Status
& .\service.ps1 -Action Logs
& .\service.ps1 -Action Stop
& .\service.ps1 -Action Start
```

`Status` reports service state and local readiness. `Logs` reads the `AphelionDMMRelay` Windows Event Log source. Public checks are:

```powershell
Invoke-RestMethod https://mapping.a13.info/v1/health/live
Invoke-RestMethod https://mapping.a13.info/v1/health/ready
Invoke-RestMethod https://mapping.a13.info/v1/version
```

## Update and rollback

Extract an approved newer ZIP and run:

```powershell
& .\service.ps1 -Action Update -SkipCloudflare
```

This retains the installed dedicated connector and token, replaces only relay-owned files, waits for readiness, and automatically restores the prior relay files if startup fails. To rotate the dedicated token and refresh the pinned connector in the same update, omit `-SkipCloudflare` and supply `-TunnelTokenFile`.

## Uninstall

```powershell
& .\service.ps1 -Action Uninstall
```

This removes only the `AphelionDMMRelay` service, its event source, and its program directory. It refuses to delete a same-named service whose executable is not the expected relay. Configuration and token data remain for recovery unless explicitly purged:

```powershell
& .\service.ps1 -Action Uninstall -PurgeData
```

## Loopback certification without Cloudflare

For a disposable administrative test, install without the connector:

```powershell
& .\service.ps1 -Action Validate -SkipCloudflare
& .\service.ps1 -Action Install -SkipCloudflare
& .\service.ps1 -Action Status
Invoke-RestMethod http://127.0.0.1:8080/v1/health/ready
& .\service.ps1 -Action Uninstall -PurgeData
```

## Recovery and privacy

After a relay restart, the owner reconnects first and recreates the room from local state; participants then reconnect. There is no server backup, restore, migration, database, or user-management operation.

Relay logs must not contain map content or paths, display names, invitations, capabilities, group keys, identity keys, or client database contents. If a tunnel token is exposed, rotate only the dedicated AphelionDMM tunnel token and reinstall it.
