# Disposable desktop hosted pilot

This stack runs the real hosted collaboration service behind trusted local TLS with PostgreSQL and a signed, interactive OIDC fixture. It is for same-machine certification only. All published ports bind to loopback, the credentials in `compose.yaml` are deliberately non-production, and `down --volumes` destroys its database and certificates.

## Start or rebuild

Run from the repository root in PowerShell:

```powershell
docker compose -f deploy/pilot/compose.yaml down --volumes --remove-orphans
docker compose -f deploy/pilot/compose.yaml up --build --detach
docker compose -f deploy/pilot/compose.yaml ps
```

Wait for both `postgres` and `hosted` to report `healthy`. The service is `https://localhost:8443`; the disposable identity provider is `https://localhost:9443`.

## Trust the disposable certificates

The browser and StrongDMM must trust the two roots generated for this run. Import them into the current Windows user's root store:

```powershell
$pilotCertDir = Join-Path $env:TEMP 'AphelionDMM-pilot-certs'
New-Item -ItemType Directory -Force -Path $pilotCertDir | Out-Null
$caddyRoot = Join-Path $pilotCertDir 'caddy-root-current.crt'
$oidcRoot = Join-Path $pilotCertDir 'oidc-root-current.crt'
docker cp apheliondmm-desktop-pilot-edge-1:/data/caddy/pki/authorities/local/root.crt $caddyRoot
docker cp apheliondmm-desktop-pilot-oidc-1:/tls/oidc-root.crt $oidcRoot
Import-Certificate -FilePath $caddyRoot -CertStoreLocation Cert:\CurrentUser\Root
Import-Certificate -FilePath $oidcRoot -CertStoreLocation Cert:\CurrentUser\Root
```

Re-import after every `down --volumes`, because that creates new roots. Remove the exact imported roots when the pilot is finished:

```powershell
$pilotCertDir = Join-Path $env:TEMP 'AphelionDMM-pilot-certs'
Get-ChildItem (Join-Path $pilotCertDir '*-current.crt') | ForEach-Object {
    $thumbprint = (Get-PfxCertificate -FilePath $_.FullName).Thumbprint
    Remove-Item -LiteralPath "Cert:\CurrentUser\Root\$thumbprint"
}
```

## Inspect and stop

```powershell
Invoke-RestMethod https://localhost:8443/v1/health/ready
docker compose -f deploy/pilot/compose.yaml logs --no-color --tail 200 hosted oidc edge postgres
docker compose -f deploy/pilot/compose.yaml down --volumes --remove-orphans
```

Do not paste invitations or authentication responses into logs or issue reports. Do not expose ports 8443, 9443, or 5432 through a router, tunnel, or public firewall rule.
