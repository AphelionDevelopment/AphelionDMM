# Phase 7 public hosting and self-hosting verification

**Date:** 2026-08-26  
**Scope:** Public default origin, hosted load authentication, portable production deployment, local container lifecycle, and Cloudflare/server inventory.  
**Excluded:** Live public deployment, production OIDC credentials, named-user pilot, Content Tools, and multi-replica operation.

## Implemented

- StrongDMM's editable hosted sign-in field defaults to `https://mapping.a13.info`.
- The load runner accepts a bounded, permission-checked JSON file containing per-editor OIDC session credentials. It creates and redeems ordinary hosted invitations and does not call `/join-tokens` in hosted mode.
- `deploy/production` provides a private PostgreSQL and hardened single-replica hosted-service base plus optional Cloudflare Tunnel and loopback reverse-proxy overlays.
- The Cloudflare overlay is public and contains no Access policy. It uses a dedicated token file and pinned `cloudflared` image.
- The operator wrapper provides initialization, validation, build, start, status, logs, backup, restore, upgrade, rollback, and stop actions.
- The hosted YAML configuration has a published JSON Schema and file-backed secret example.

## Cloudflare inventory

- `mapping.a13.info` was unoccupied at inspection time.
- The existing remotely managed `bark` tunnel was healthy and remained unchanged.
- The existing connector runs Cloudflared 2026.7.3 on Windows AMD64 at the Meridian server's public IP.
- The selected design uses a separate public tunnel without a Cloudflare Access application.
- No tunnel, DNS record, Access application, or public route was created during this verification.

## Local production lifecycle

Both production Compose overlays passed `docker compose config --quiet`. The Cloudflare 2026.7.3 image's shipped help confirmed `--token-file`, and the image was pinned to its pulled repository digest.

The loopback production stack then completed its real operator path:

1. build the distroless hosted image;
2. start PostgreSQL and wait for its final TCP listener;
3. start the hosted service and reach container health;
4. return `{"status":"ok"}` from `/v1/health/ready`;
5. return the expected build, revision, protocol, and schema data from `/v1/version`;
6. create a PostgreSQL custom-format backup;
7. stop the hosted service, restore that backup with explicit destructive confirmation, and recover readiness;
8. stop the service and replace it with a retained known-good image tag;
9. recover readiness and shut down both services cleanly.

The first start exposed a Docker Desktop portability defect: read-only Compose secrets mounted from Windows report synthetic mode `0777`, while the resolver required Unix `0600`. A regression test and narrow fix now allow only regular, non-symlink files under `/run/secrets` whose mount rejects an open-for-write probe. Ordinary secret paths retain the original permission rule. The rebuilt stack passed.

Disposable validation credentials and the local backup were removed after the stack stopped. No production-validation containers or PostgreSQL volume remain. The separate desktop-pilot stack was left running and was not modified by this verification.

## Automated and build evidence

- Focused UI, load, server, container, and load-command packages passed.
- `go test ./... -count=1` passed.
- `go test -race ./internal/aphelion/... -count=1` passed.
- The pinned Rust 1.82 Windows build completed after adding the already installed Go bin directory to the process-local `PATH`.
- `dst/StrongDMM.exe` SHA-256: `E04176F1CFFF1D4D7164D754A19947F6CEDA96887FF93C0BFF00F233B87EB19A`.
- The inherited ImGui/GCC `memset` warning remains unchanged.

## Remaining external gates

- Complete interactive Cloudflare Access login for read-only Meridian server inventory.
- Confirm server Docker capacity, storage, backup destination, and service account.
- Register the real OIDC client and store its client ID and secret.
- Create the dedicated tunnel, install its token on the server, and publish `mapping.a13.info` only after local readiness.
- Run the hosted 25-editor load/fault profile with disposable OIDC identities.
- Exercise public OIDC, WebSocket reconnect, restart, backup restore, rollback, telemetry, and named-user pilot flows.
- Assign updater minisign ownership before production updater publication.
