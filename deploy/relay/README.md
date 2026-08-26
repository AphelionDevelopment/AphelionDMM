# AphelionDMM protocol-v2 relay

This directory runs the stateless public relay used by client-owned collaboration. The relay stores no maps, operation history, accounts, profiles, or database. Restarting it discards rooms; owner clients recreate them from local state.

## Requirements

- Docker Engine with Docker Compose v2 support
- An immutable, approved AphelionDMM source revision
- For public service, a dedicated remotely managed Cloudflare Tunnel token
- No inbound origin firewall opening

The Cloudflare route is configured in the Cloudflare dashboard:

```text
mapping.a13.info -> http://relay:8080
```

Do not create a Cloudflare Access application for this hostname. Invitations and protocol signatures provide session admission; users should not need membership in the operator's Cloudflare account.

Cloudflare documents token-file support for remotely managed tunnels and published HTTP hostname routing:

- https://developers.cloudflare.com/tunnel/advanced/run-parameters/#token-file
- https://developers.cloudflare.com/tunnel/routing/

## Files

- `relay.yaml`: the only application configuration; created from `relay.yaml.example`.
- `.env`: local image/build metadata; created from `.env.example`.
- `secrets/cloudflare_tunnel_token.txt`: the only deployment secret and only for the tunnel sidecar.
- `compose.yaml`: hardened relay service with no published port or volume.
- `compose.loopback.yaml`: publishes `127.0.0.1:8080` for local verification.
- `compose.cloudflare.yaml`: adds the dedicated tunnel sidecar.
- `operations.ps1`: the supported operator entry point.

Keep `relay.yaml`, `.env`, and `secrets/` local and untracked. Never put a tunnel token in the YAML or environment file.

## First setup

From the repository root:

```powershell
& .\deploy\relay\operations.ps1 -Action Setup
```

Review `deploy/relay/relay.yaml`. Change the public origin for a self-hosted relay and choose a non-conflicting Docker subnet in both `compose.yaml` and `trusted_proxy_cidrs` before first start. Keep `bind_address` at `0.0.0.0:8080` inside the container.

Validate and build:

```powershell
& .\deploy\relay\operations.ps1 -Action Validate -Edge Loopback
& .\deploy\relay\operations.ps1 -Action Build -Edge Loopback
```

## Loopback certification

```powershell
& .\deploy\relay\operations.ps1 -Action StartLoopback
& .\deploy\relay\operations.ps1 -Action Status -Edge Loopback
Invoke-RestMethod http://127.0.0.1:8080/v1/health/live
Invoke-RestMethod http://127.0.0.1:8080/v1/health/ready
Invoke-RestMethod http://127.0.0.1:8080/v1/version
& .\deploy\relay\operations.ps1 -Action Logs -Edge Loopback
& .\deploy\relay\operations.ps1 -Action Stop -Edge Loopback
```

Confirm the relay log contains no map text, local paths, display names, invitations, capabilities, group keys, or identity keys.

## Public Cloudflare start

Create a dedicated remotely managed tunnel and published hostname in Cloudflare. Do not reuse another application's tunnel. Install its token directly into:

```text
deploy/relay/secrets/cloudflare_tunnel_token.txt
```

Restrict the file ACL to the deployment operator and Docker-running account. Do not print the token into terminal history or agent output.

Then run:

```powershell
& .\deploy\relay\operations.ps1 -Action Validate -Edge Cloudflare
& .\deploy\relay\operations.ps1 -Action StartCloudflare
& .\deploy\relay\operations.ps1 -Action Status -Edge Cloudflare
Invoke-RestMethod https://mapping.a13.info/v1/health/ready
```

The relay has no public Docker port in this mode. The tunnel reaches it on the private Compose network.

## Routine operation

```powershell
& .\deploy\relay\operations.ps1 -Action Status -Edge Cloudflare
& .\deploy\relay\operations.ps1 -Action Logs -Edge Cloudflare
& .\deploy\relay\operations.ps1 -Action Update -Edge Cloudflare
& .\deploy\relay\operations.ps1 -Action Stop -Edge Cloudflare
```

`Update` tags the prior local image as `apheliondmm-relay:rollback-<timestamp>`, rebuilds the relay, replaces only the relay service, and waits for health. To roll back, set `APHELIONDMM_RELAY_IMAGE_TAG` in `.env` to the retained tag and start the selected edge again. Record the approved source revision and image ID before and after an update.

There are no backup, restore, migration, database, or user-management operations.

## Self-hosting

A self-hoster can use the same files with another HTTPS hostname:

1. Change `public_origin` in `relay.yaml`.
2. Configure an HTTPS reverse proxy or a dedicated Cloudflare Tunnel to `http://relay:8080`.
3. Put the proxy container on the `edge` network.
4. Set `trusted_proxy_cidrs` only to that private network.
5. Keep the relay port unpublished, or use the loopback overlay behind a host-local reverse proxy.
6. Configure clients with the exact HTTPS origin.

The relay cannot fetch certificates, maps, URLs, or repositories and does not need access to the host filesystem.

## Recovery and incidents

After a relay restart, owners reconnect first and republish room admissions; participants reconnect afterward. Client databases are the recovery authority. Do not add a volume or restore procedure.

If the public route fails, disable only this tunnel route. If the relay release fails, use the retained image tag. If the tunnel token is exposed, rotate only that dedicated token and replace its file. Preserve sanitized client logs for revision/hash investigations.

