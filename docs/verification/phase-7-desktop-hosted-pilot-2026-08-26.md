# Desktop hosted pilot verification

Date: 2026-08-26

This record covers the replay/live overlap repair, recoverable client integrity handling, session-scoped names, verifier-bound desktop OIDC handoff, hosted desktop lifecycle, and the disposable TLS/PostgreSQL/OIDC pilot stack. It is local automated evidence. The remaining gate is human interaction with two StrongDMM windows; it is not an external-provider or public-internet certification.

## Environment

- Repository base revision: `3fde584d1f3ee073b414b9e9805976cba74626e5`
- Go: `go1.25.13 windows/amd64`
- Docker Engine: `29.7.2`
- Docker Compose: `v5.4.0`
- PostgreSQL: `postgres:18.6-bookworm@sha256:1c59e2c3c818eaa0f0628f695b36e7c9e362d6b219b36a54a32df645cbd7e1af`
- Caddy: `caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d`

## Verified behavior

- A deterministic WebSocket test reproduced the replay/live overlap duplicate and passes with replay bounded to the joined snapshot high-water revision.
- Exact accepted duplicates are idempotent; altered duplicates and revision gaps suspend the executor and force transport recovery.
- Owner and participant display names are validated, broadcast through authenticated presence, and hosted names are persisted by session actor identity.
- Already-closed transports are ignored only during deliberate leave; unexpected close errors remain visible.
- Desktop OIDC uses a random verifier challenge, an opaque handoff ID, a five-minute expiry, constant-time verifier comparison, and single-use exchange. The browser callback never returns the credential.
- A rejected or canceled browser callback invalidates its pending desktop handoff, so the desktop receives an immediate failed exchange instead of polling until timeout.
- Hosted invitations are versioned, HTTPS-bound, bounded, one-use, expiry-checked by the desktop decoder, and redeemed only after OIDC sign-in.
- The local pilot publishes only `127.0.0.1:8443` and `127.0.0.1:9443`; PostgreSQL and the hosted process have no host-published port.
- The interactive test OIDC page issues distinct signed subjects and names for separate desktop windows.
- PostgreSQL readiness probes the final TCP listener rather than the temporary initialization Unix socket, closing the observed startup refusal window.

## Gates run

- `go test ./...`: pass, with the inherited imgui GCC `memset` warning.
- `go test -race ./internal/aphelion/collab/...`: pass, with the same inherited warning.
- `go test -tags container ./internal/aphelion/collab/container -run TestHostedImageLifecycle -count=1`: pass against the locally built hosted image.
- `go vet ./internal/aphelion/collab/...`: pass, with the inherited imgui compiler warning.
- `npx --yes @redocly/cli@2.18.0 lint api/collaboration/openapi.yaml`: valid; four pre-existing operation-response style warnings.
- `npx --yes @asyncapi/cli validate api/collaboration/asyncapi.yaml`: valid; one informational recommendation to move from AsyncAPI 3.0.0 to 3.1.0.
- `docker compose -f deploy/pilot/compose.yaml config`: pass.
- Live TLS desktop handoff: identity form, callback completion, verifier exchange, and logout passed without printing the credential.
- Real Windows `task build` with `RUST_TARGET=1.82.0-x86_64-pc-windows-gnu`: pass, with the inherited imgui warning.
- `trivy image --scanners vuln,secret,misconfig --severity HIGH,CRITICAL --exit-code 1 --no-progress apheliondmm-desktop-pilot-hosted:latest`: pass with zero findings in the Debian layer and both Go binaries.
- `go vet ./...`: not green because of four existing `unsafe.Pointer` diagnostics in `internal/platform/gl.go`; no new collaboration package diagnostic was emitted.

The earlier transient PostgreSQL refusal was traced to the image's two-stage initialization. A hostless `pg_isready` probe could report the temporary Unix-socket-only server healthy immediately before the entrypoint stopped it and launched the final TCP server. Both the Compose stack and container harness now probe `127.0.0.1:5432`. Three unchanged lifecycle reproductions passed before the correction; the corrected configuration test, Compose validation, and lifecycle against the rebuilt image then passed. The rebuilt local stack is healthy and its last 200 log lines contain no panic, fatal, connection-refused, error, or failed matches.

The rebuilt TLS stack also passed a live canceled-browser flow: readiness returned `ok`, the canceled callback returned HTTP 401, and the subsequent verifier exchange returned HTTP 401 without producing a credential.

Final artifacts:

- `dst/StrongDMM.exe`: `6048197FEEBA2ACE51BD52CBDA30087449C071830BD2C016C5C94438BD9C8E92`
- hosted image ID: `sha256:df38122c08d2253f5b16dd8a27ca6c54476a70ca340b28c72e90fe1d897cb49c`
- interactive OIDC fixture image ID: `sha256:a8c85fa0ffd4a597ac4c10f1514a5d425878bd472570d11819ecff7159bf5906`

## Human-only gate

Follow `docs/testing/multiplayer-online-pilot-guide.md` using two StrongDMM windows. Judge browser usability, simultaneous visible edits, presence, conflict clarity, reconnect behavior, rename propagation, and final saved-map convergence. External OIDC registration, public DNS/TLS, telemetry, firewall policy, rollback rehearsal, and named remote users remain operator inputs for a later private network deployment.
