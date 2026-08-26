# Client-owned relay automated acceptance (superseded deployment evidence)

> Protocol and client evidence in this record remains valid. Its Docker deployment evidence was superseded on 2026-08-27 by `windows-relay-service-automated-2026-08-27.md` and is retained only as historical verification.

**Date:** 2026-08-26  
**Scope:** Protocol-v2 client authority, opaque relay, desktop integration, load/fault gates, protected container/operator bundle, and CI entry points  
**Excluded:** Public deployment, two-network human testing, Content Tools, desktop owner-transfer UI, release publication, commit, and push

## Result

The protocol-v2 implementation is ready to stage at `mapping.a13.info` and then enter the public two-network human pilot. Local automated acceptance passed. This report does not claim that the public hostname is deployed or externally tested.

Manual reconnect is the accepted recovery control for the first pilot. Desktop owner transfer is deferred from first-pilot scope; its protocol, authority, and relay validation exist, but its complete desktop offer/accept flow does not.

## Repository and artifacts

- Working branch: `codex/client-owned-relay`
- Changes: uncommitted and unpushed
- `StrongDMM.exe` SHA-256: `E8DD878CFDDAB5CD400D22123825AC5340F28C414292497D7A178549459741D7`
- `apheliondmm-relay.exe` SHA-256: `74E8F5CEED2739ECDC1091D1D47B2EA73FB217D07B41505735B7199C4372D090`
- `apheliondmm-healthcheck.exe` SHA-256: `6260BDDEE17025CE23272E751D857E98A87A7B247765355ACB8C7A4916E97157`
- Relay image ID: `sha256:85ed4554a1ee98441d46f485ffb251b960305e075cb155e4d32ff1ffde2b7116`
- Docker Engine: 29.7.2
- Docker Desktop: 4.88.1
- Docker Compose: v5.4.0

## Automated gates

The following commands completed successfully with exit code 0:

```powershell
go test ./... -count=1
go test -race ./internal/aphelion/... -count=1
go test ./internal/aphelion/buildcheck
task verify-relay
task test-rust
task build
task build-relay
actionlint .github/workflows/ci.yml
govulncheck ./...
```

Protocol-v2 fuzzing completed without a failure:

- `FuzzUnmarshalEnvelope`, 5 seconds, approximately 636,000 executions.
- `FuzzDecodeApplication`, 5 seconds, approximately 408,000 executions.

The race-enabled relay load runner passed repeated 2-, 8-, and 32-participant convergence runs. A bounded admission-publication retry repaired the ordering race found during this gate.

## Container and operator evidence

The pinned distroless image runs as `nonroot:nonroot`, uses a read-only root filesystem, drops all capabilities, sets `no-new-privileges`, and mounts no map, database, repository, or Docker socket. Static tests reject PostgreSQL, OIDC, database secrets, backup volumes, and public relay-port publication in the Cloudflare overlay.

`TestRelayImageLifecycle` exercised the exact image with two encrypted protocol clients, health/readiness/version checks, opaque traffic routing, relay-only restart, reconnect, hardening inspection, and a log plaintext assertion. It passed.

The singular operator entry point passed `Setup`, `Validate`, `Build`, `StartLoopback`, `Status`, health/version requests, `Logs`, `Update`, and `Stop`. `Update` force-recreated the relay, retained a timestamped rollback image tag, and replaced the container. Cloudflare Compose validation passed using a temporary placeholder token that was deleted immediately afterward.

The local operator files `deploy/relay/.env`, `deploy/relay/relay.yaml`, and `deploy/relay/secrets/` are ignored. No production tunnel token was created, retrieved, or stored by this acceptance run.

## Security and dependency results

`govulncheck ./...` found no reachable vulnerability. It reported six vulnerabilities in required modules whose vulnerable functions are not reached by this program. Trivy reported zero HIGH or CRITICAL findings across OS packages, both Go binaries, secrets, and image misconfiguration.

The inherited ImGui C++ `memset` warning remains present during the Windows build. It is unchanged and did not fail the build.

## Cloudflare pre-deployment inventory

Read-only inventory found:

- active, unpaused `a13.info` zone;
- no DNS record for `mapping.a13.info`;
- no account- or zone-level Cloudflare Access application for `mapping.a13.info`;
- one existing healthy tunnel named `bark`, which is out of scope and must remain unchanged.

Production therefore requires a new dedicated remotely managed tunnel, recommended name `apheliondmm-mapping`, routing `mapping.a13.info` to `http://relay:8080` without Cloudflare Access.

## Remaining acceptance

1. Install an authorized immutable revision on the dedicated host using `deploy/relay/operations.ps1`.
2. Create the dedicated tunnel and install its token directly on that host without printing or committing it.
3. Publish `mapping.a13.info`, then verify public health, readiness, version, and WebSocket routing from an external network.
4. Run `docs/testing/client-owned-relay-human-test-guide.md` with two computers on separate networks.
5. Record revisions, hashes, reconnect behavior, desync recovery, saved-map fidelity, UI/accessibility observations, and sanitized relay/client logs.

A public pilot pass is still required before multiplayer can be called complete or protocol v2 can replace protocol v1 in release/deployment defaults.
