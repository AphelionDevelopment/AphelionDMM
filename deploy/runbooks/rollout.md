# Hosted collaboration staged rollout

## Entry criteria for every stage

- repository, race, PostgreSQL integration, compatibility, vulnerability, image, backup/restore, OIDC revocation, and cross-repository integration gates pass for the exact build;
- rollback uses the exact prior image/build and a compatible or restored database;
- public discovery is disabled and every participant is named and invited;
- dashboards/alerts cover acknowledgement latency, reconnects, conflicts, database health, authorization rejection, and recovery state;
- no open severity-one incident or known data-loss/divergence condition.

## Current deployment contract

- Run exactly one hosted-service replica. PostgreSQL provides durable shared state, but cross-instance accepted-operation fanout is not implemented; overlapping replicas could leave clients on one replica unaware of writes accepted by another.
- Use a stop-then-start (`Recreate`) application update. Do not claim a zero-downtime rolling upgrade until a tested cross-instance event transport exists.
- Terminate TLS at a trusted reverse proxy. The proxy must replace, not append to attacker-supplied, `X-Forwarded-For`; its network must be listed in `trusted_proxy_cidrs`. The application ignores forwarded addresses from every other peer.
- Run the image as non-root with a read-only root filesystem, all Linux capabilities dropped, `no-new-privileges`, and the configuration mounted read-only at `/etc/apheliondmm/config.yaml`. The service requires no writable application-data volume.
- Configure the orchestrator readiness and liveness probes against `/v1/health/ready` and `/v1/health/live`. The image health check defaults to loopback port 8080 and can be changed with `APHELIONDMM_HEALTHCHECK_URL` when the bind port differs.

## Stage 0: internal loopback

Run the produced service entry point locally against disposable PostgreSQL and the signed local OIDC issuer. Exercise login/logout, invitation redemption, two-client editing, restart recovery, and clean shutdown. Record build and configuration hashes.

## Stage 1: private hosted pilot

Use named Aphelion developers only. Run the recorded 25-editor load/fault profile before inviting users. Hold at this stage until backup/restore and stop-then-start rollback rehearsals meet targets and no authorization bypass, acknowledged loss, or divergence occurs.

## Stage 2: wider opt-in pilot

Expand only with explicit incident review and owner approval. Preserve invitation-only access, support rollback for the full compatibility window, and compare p50/p95/p99 acknowledgement latency, reconnect rate, conflicts, and store health with Stage 1.

## Stop conditions

Immediately stop expansion on acknowledged-operation loss, divergent hashes, authorization bypass, unrecoverable compatibility failure, failed restore, unsigned/unverifiable artifact, or p95 acknowledgement above the approved gate during the reference run. Follow `incident-response.md`; expansion resumes only after new evidence and explicit approval.
