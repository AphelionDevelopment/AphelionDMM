# AphelionDMM Windows Relay Service Design

**Status:** Approved implementation baseline  
**Date:** 2026-08-27  
**Supersedes:** The protocol-v2 Docker and Compose deployment described on 2026-08-26  
**Preserves:** Client-owned durable state, stateless relay routing, opaque payloads, dedicated Cloudflare Tunnel, and public `mapping.a13.info` without Cloudflare Access

## Decision

Protocol-v2 production hosting uses a native Windows service and does not require Docker. The installed `AphelionDMMRelay` service runs the relay in-process and supervises one pinned `cloudflared.exe` child. A child-process failure terminates the service so Windows Service Control Manager recovery restarts the complete pair.

The service has a unique name and never creates, changes, stops, or reuses the existing `Cloudflared` service or the `bark` tunnel. This is necessary because Cloudflare's built-in Windows installer uses a fixed service name. The dedicated child process receives only a token-file path, never the token value on its command line.

## Installation layout

```text
C:\Program Files\AphelionDMM Relay\
  apheliondmm-relay.exe
  apheliondmm-healthcheck.exe
  cloudflared.exe

C:\ProgramData\AphelionDMM\Relay\
  relay.yaml
  cloudflare_tunnel_token.txt
```

`relay.yaml` is the only non-secret configuration. The tunnel token remains a separate ACL-protected secret. The relay and metrics bind only to loopback. No inbound firewall rule is created.

The service runs as `NT AUTHORITY\LocalService`, starts automatically with delayed startup, accepts stop and shutdown controls, and records lifecycle messages in the Windows Application event log. Service recovery restarts it after an unexpected exit with bounded delays and a 24-hour failure-count reset.

## Runtime

The console relay and service share one relay runtime. Service startup performs these steps:

1. Load the strict relay YAML and start the HTTP/WebSocket listener.
2. Wait for the loopback readiness endpoint.
3. Start the pinned Cloudflare connector with `tunnel run --token-file <path>`.
4. Remain running while both components are healthy.
5. On stop or shutdown, cancel the connector and gracefully shut down the relay.
6. If either component exits unexpectedly, stop the other component and return an error to SCM.

The relay remains independently runnable in console mode without `cloudflared` for development and loopback certification.

## Installer and operator interface

`deploy/relay/service.ps1` is the only supported entry point. It exposes:

- `Setup`: create a local package workspace and example configuration.
- `Validate`: validate YAML, package files, architecture, hashes, ACL expectations, and service-name conflicts.
- `Package`: build a self-contained Windows x64 ZIP for a machine without Go.
- `Install`: require elevation, install files and ACLs, register the event source and service, configure recovery, and start after validation.
- `Start`, `Stop`, `Status`, and `Logs`: operate only `AphelionDMMRelay`.
- `Update`: stage and validate a new package, stop the service, retain a rollback directory, replace binaries, start, health-check, and automatically restore on failure.
- `Uninstall`: remove only the service and event source; preserve configuration and the tunnel token unless `-PurgeData` is explicitly supplied.

An interactive install accepts the tunnel token through a secure prompt. Automation may supply a regular, non-link token file. The script never prints the token. It downloads pinned Cloudflare `cloudflared` 2026.7.3 for Windows x64, verifies SHA-256 `8635da433b6df8194746e88ed9d2589566c20e38bfc2a80e431a348b7c765841`, and requires a valid Authenticode signature.

## Security boundary

- No database, map directory, Docker socket, container runtime, OIDC secret, backup job, or public origin port.
- Program Files binaries are writable only by Administrators and SYSTEM.
- ProgramData configuration and token are readable only by Administrators, SYSTEM, and LocalService; the token is never executable.
- The service binary accepts fixed deployment flags only. Protocol messages never carry filesystem paths or process arguments.
- The connector binary has automatic updates disabled. Version changes require a reviewed pin, checksum, package rebuild, and normal update operation.
- The installer rejects reparse points and refuses to operate on a service whose executable is outside the AphelionDMM installation root.

## Packaging and CI

Linux CI retains protocol fuzz, race, command, and cross-platform tests. A Windows CI job builds the service package, validates the pinned Cloudflare binary, installs the relay in loopback-only test mode, checks health/version, restarts the service, checks health again, stops and uninstalls it, and uploads the ZIP artifact. Filesystem vulnerability and secret scans replace the protocol-v2 container scan.

Legacy protocol-v1 Docker material remains historical and unchanged. Only `deploy/relay` loses Docker files.

## Acceptance

Automated acceptance requires service-host unit tests, native Windows compilation, package validation, real SCM install/start/restart/stop/uninstall, repository Go/race/Rust/editor gates, dependency and secret scans, and tracked privacy checks.

Human acceptance begins after the package is installed on the dedicated Windows host, the dedicated tunnel is connected, `mapping.a13.info` is externally healthy, and the existing `bark` tunnel is confirmed unchanged.
