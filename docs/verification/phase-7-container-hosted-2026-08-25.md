# Phase 7 hosted container and local OIDC verification

Date: 2026-08-25

## Scope

This record covers the locally built hosted OCI image, its hardened runtime settings, a signed disposable OIDC provider, PostgreSQL-backed hosted lifecycle, restart and database-loss recovery, and image scanning. It is local Docker Desktop evidence, not a public deployment, external identity-provider certification, multi-replica proof, or private-user pilot.

## Image evidence

The image was built from the digest-pinned Go 1.25.13 Bookworm builder and distroless Debian 13 non-root runtime in `deploy/container/Dockerfile`:

```text
docker build --file deploy/container/Dockerfile --build-arg APHELIONDMM_BUILD=local-verification-3 --build-arg APHELIONDMM_REVISION=763e341fcd0efacd1b19ce013645564144d5cb6f --tag apheliondmm-hosted:local-verification .
```

Result:

- image manifest-list ID: `sha256:2584c4b4c0d50d251ed4a492d950e0e974d028f627e3a0962c5367bd1da986ef`;
- compressed size reported by Docker: 10,739,202 bytes;
- runtime user: `nonroot:nonroot`;
- entry point: `/apheliondmm-hosted`;
- the runtime layer contains the hosted binary and compiled health-check binary, with no shell or package manager.

The runtime test asserted a read-only root filesystem, all Linux capabilities dropped, `no-new-privileges`, read-only configuration and CA mounts, a bounded `noexec,nosuid` temporary filesystem, and a loopback-only published port.

## Automated lifecycle gate

The protected hosted CI job now runs the Docker-tagged test after building the exact image:

```text
$env:APHELION_CONTAINER_TEST_IMAGE = 'apheliondmm-hosted:local-verification'
go test -tags=container ./internal/aphelion/collab/container -run '^TestHostedImageLifecycle$' -count=1 -timeout=5m -v
```

The final local run passed in 7.14 seconds. The gate starts unique disposable containers and a private TLS issuer/OTLP sink, then verifies:

- OIDC discovery and confidential Authorization Code with S256 PKCE against a locally generated CA;
- signed token, issuer, audience, nonce, state, and callback handling through the running image;
- separate owner and editor provider subjects;
- one-use hosted invitation redemption and rejection on reuse;
- raw invitation material is returned only to the caller while PostgreSQL stores a 32-byte hash;
- snapshot access by both durable hosted members;
- graceful application shutdown with exit code 0;
- fresh login and exact snapshot recovery after application restart;
- readiness changes to HTTP 503 while PostgreSQL is stopped and the application process remains running;
- readiness returns to HTTP 200 after PostgreSQL recovers;
- logout invalidates the hosted authentication session.
- configured OTLP/HTTP trace and metric exporters send protobuf payloads over verified TLS and flush during graceful shutdown.

The test owns unique resource names and removes only those exact containers and network during cleanup. No image is pushed and no external identity or hosting service is contacted.

## Authorization boundary repair

A hosted owner could previously call the embedded `/join-tokens` endpoint and mint a process-local identity outside durable OIDC membership. A regression test first reproduced HTTP 201. Hosted credentials are now rejected by that embedded-only endpoint with HTTP 403; hosted sessions must use hashed, one-use hosted invitations. The OpenAPI description records the boundary.

## Security scans

Local Trivy 0.74.0 completed both gates:

```text
trivy image --scanners vuln,secret,misconfig --severity HIGH,CRITICAL --exit-code 1 --no-progress apheliondmm-hosted:local-verification
trivy config --severity HIGH,CRITICAL --exit-code 1 deploy/container
```

Results: zero high/critical Debian or Go-binary vulnerabilities, zero detected secrets, and zero Dockerfile misconfigurations. CI separately uses the commit-pinned Trivy action recorded in `.github/workflows/ci.yml`.

## Remaining hosted gates

- exercise a supported external non-production OIDC provider;
- run the recorded 25-editor profile against a reference hosted deployment and record p50/p95/p99;
- inject a durable slow consumer and sustained presence flood during that reference run, then prove final revision/hash convergence;
- rehearse stop-then-start rollback to the exact prior image and compatible database state;
- provision the human-controlled updater minisign key and verify its signed manifest/artifacts before publication; GitHub/Sigstore release provenance is now configured separately;
- implement cross-instance event fanout before permitting more than one hosted-service replica;
- deploy a collector plus dashboards/alerts and exercise exporter authentication policy for the selected environment;
- run named private-user and cross-repository acceptance pilots.
