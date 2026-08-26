# Online Pilot Readiness Design

**Date:** 2026-08-26
**Status:** Approved

## Objective

Make the StrongDMM collaboration client safe for a private internet-hosted pilot by repairing the observed revision desynchronization, adding session-scoped participant names, and exposing the existing hosted PostgreSQL/OIDC service through a native desktop workflow.

## Confirmed defects

The 2026-08-25 two-instance test recorded `accepted revision is 9, want 10`. The server subscribes a joining connection to live durable operations before loading replay. An operation committed inside that overlap can consequently be sent once by replay and again from the live queue. The client currently treats that exact duplicate as terminal corruption and never initiates recovery.

Embedded session creation also constructs its owner principal with the literal display name `Owner`. The desktop provides a name field for invitees but no equivalent owner or self-profile control. A normal WebSocket close during application teardown is surfaced as an error even though the connection is already closed.

## Workstream A: convergence and identity repair

The server establishes a replay high-water revision from the authoritative snapshot used for the join. Replay sends only operations newer than the client's acknowledged revision and no newer than that high-water. After replay completion, queued live operations at or below the high-water are discarded; newer operations retain FIFO order. This closes the replay/live overlap without risking a missed operation.

The client additionally accepts a repeated authoritative operation only when its operation ID, accepted operation, revision, and authoritative hash exactly match a result already applied. Unknown, changed, or hash-mismatched old revisions remain integrity failures. A non-duplicate integrity failure closes the active transport and enters the existing reconnect/snapshot-fallback path instead of leaving the editor permanently terminal.

All participants receive a session-scoped display name. Embedded owners choose an initial name when starting a session. The collaboration panel exposes the current name and an authenticated rename action. The server updates the active hub member and presence record; embedded credentials for the same actor are updated so reconnect does not restore a stale name. Hosted membership stores the nickname separately from immutable OIDC issuer/subject identity.

Normal close errors (`net.ErrClosed`, WebSocket normal closure, or an already-closed connection) are ignored during deliberate leave/project teardown. Unexpected close errors remain logged and visible.

## Workstream B: native hosted desktop flow

The existing `apheliondmm-hosted` service remains the only internet-facing collaboration mode. Embedded mode stays loopback-only and must not be port-forwarded.

StrongDMM gains a configured hosted service origin and these user flows:

1. **Sign in:** the desktop begins authentication and opens the returned authorization URL in the system browser. The backend callback completes OIDC and a short-lived single-use handoff, bound to a desktop-generated verifier, returns the hosted credential to the polling desktop. Tokens are never displayed in browser HTML, URLs, logs, or clipboard content.
2. **Start hosted session:** an authenticated client uploads the active map snapshot to `/v1/hosted/sessions`, connects using its hosted credential, and exposes hosted invitation creation in the existing panel.
3. **Join hosted session:** an encoded invitation identifies the HTTPS origin, session, and one-time invitation secret. After sign-in, the desktop redeems it to establish membership and connects with its hosted credential.
4. **Sign out:** the client invalidates the hosted credential and clears it from memory. Credentials are not persisted in StrongDMM configuration.

The hosted pilot uses HTTPS/WSS at a trusted reverse proxy, PostgreSQL, a non-production OIDC tenant, explicit trusted proxy CIDRs, no public discovery, and named invited testers. The first certification uses the existing signed local OIDC fixture and disposable PostgreSQL containers. Live deployment later requires operator-provided DNS, TLS, database, and OIDC values.

## Security and failure behavior

- Cleartext non-loopback HTTP and WebSocket connections remain rejected.
- Browser handoffs expire quickly, are single use, are rate limited, and require the unguessable desktop verifier.
- Invitation secrets and hosted credentials are redacted from errors and logs.
- An integrity mismatch stops edits until authoritative recovery succeeds.
- Pending speculative edits are never silently discarded. If snapshot fallback cannot preserve them, they remain explicit conflicts requiring refresh, discard, or rebuild.
- Public session listing and anonymous session creation remain unavailable.

## Verification gates

- Deterministic regression test reproduces an operation accepted during replay and proves one delivery to the client.
- Exact duplicate acceptance is idempotent; altered duplicates remain terminal.
- Forced non-duplicate divergence reaches reconnect and snapshot recovery.
- Owner create/rename/reconnect and participant rename/presence tests pass.
- Hosted browser handoff tests cover expiry, replay, wrong verifier, cancellation, and secret redaction.
- Two real StrongDMM processes pass local editing, reconnect, naming, undo/redo, and large-map checks.
- Hosted container integration passes against PostgreSQL and the signed OIDC fixture before any public endpoint is used.

