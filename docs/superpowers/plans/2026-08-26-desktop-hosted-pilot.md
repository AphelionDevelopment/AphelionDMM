# Desktop Hosted Pilot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect real StrongDMM desktop instances to the existing hardened hosted service through safe OIDC sign-in, hosted session creation, and invitation redemption.

**Architecture:** Add a verifier-bound, single-use browser-to-desktop handoff to the hosted authentication backend. Keep hosted credentials in memory, expose hosted lifecycle methods through `SessionClient` and `Controller`, and reuse the existing authoritative WebSocket executor after membership creation.

**Tech Stack:** Go 1.24, OIDC authorization code plus PKCE, PostgreSQL, HTTPS/WSS, StrongDMM ImGui UI, OCI containers.

**Spec:** `docs/superpowers/specs/2026-08-26-online-pilot-readiness-design.md`

> **Status reconciliation (2026-08-26):** The verifier-bound desktop handoff, in-memory hosted client, StrongDMM menu flow, versioned hosted invitations, and disposable TLS/PostgreSQL/interactive-OIDC stack are implemented. Canceled browser callbacks now invalidate pending desktop handoffs, hosted invitation decoding rejects expired or missing expiry data, and PostgreSQL readiness probes the final TCP listener. OpenAPI, AsyncAPI, repository, race, collaboration vet, container, live handoff, and Windows build gates pass. The service origin is deliberately entered per sign-in instead of persisted, so no hosted credential or endpoint preference is written to disk. Remaining items are the two-window human pilot and later operator-provided external DNS/TLS/OIDC/telemetry inputs. Changes remain uncommitted.

## Global Constraints

- Internet collaboration uses only the hosted service over HTTPS/WSS.
- Hosted credentials and handoff secrets are never placed in URLs, logs, configuration, or clipboard invitations.
- Public discovery stays disabled; every pilot participant is explicitly invited.
- Preserve existing embedded mode and protocol semantics.
- Use test-driven development and do not commit or push.

---

### Task 1: Verifier-bound desktop authentication handoff

**Files:**
- Modify: `internal/aphelion/collab/auth/session.go`
- Modify: `internal/aphelion/collab/server/hosted.go`
- Modify: `internal/aphelion/collab/server/http.go`
- Modify: `api/collaboration/openapi.yaml`
- Test: `internal/aphelion/collab/auth/session_test.go`
- Test: `internal/aphelion/collab/server/hosted_test.go`

**Interfaces:**
- Produces: begin request with verifier challenge; opaque handoff ID; polling exchange authenticated by verifier; single-use in-memory hosted credential result.

- [ ] Add expiry, wrong-verifier, replay, cancellation, and successful-exchange tests.
- [ ] Confirm each test fails against current `/v1/auth/begin` behavior.
- [ ] Implement bounded handoff storage and generic OIDC callback completion without returning tokens in callback output.
- [ ] Update OpenAPI and pass its validation gate.

### Task 2: Desktop hosted client API

**Files:**
- Create: `internal/aphelion/collab/ui/hosted_client.go`
- Test: `internal/aphelion/collab/ui/hosted_client_test.go`
- Modify: `internal/aphelion/collab/ui/session_client.go`

**Interfaces:**
- Produces: `SignInHosted`, `CreateHosted`, `RedeemHostedInvitation`, and `SignOutHosted` methods; credentials held only in memory.

- [ ] Write HTTP contract tests with a real `httptest.Server` for every hosted lifecycle operation and redacted failure.
- [ ] Implement strict HTTPS-origin validation, bounded responses, and no redirects.
- [ ] Reuse `SessionClient.Join` after authenticated membership is established.

### Task 3: StrongDMM hosted session UI

**Files:**
- Modify: `internal/app/action_user.go`
- Modify: `internal/app/ui/menu/menu.go`
- Modify: `internal/aphelion/collab/ui/panel.go`
- Modify: StrongDMM configuration files under `internal/app/config` as established by nearby settings.
- Test: corresponding app/UI tests.

- [ ] Add tests for configured/unconfigured hosted origin, sign-in cancellation, session creation, invitation redemption, and sign-out.
- [ ] Add menu actions for sign in, start hosted session, join hosted session, and sign out.
- [ ] Keep hosted credentials out of persisted configuration while persisting only the approved service origin.
- [ ] Show precise state without exposing credentials.

### Task 4: Hosted invitation format

**Files:**
- Modify: `internal/aphelion/collab/ui/invitation.go`
- Test: `internal/aphelion/collab/ui/invitation_test.go`

- [ ] Add a versioned hosted invitation variant containing HTTPS origin, session ID, one-time invitation secret, and expiry.
- [x] Prove decoding rejects cleartext origins, unknown fields, oversized payloads, and malformed/expired invitations.
- [ ] Ensure logs and error strings never serialize the invitation.

### Task 5: Local hosted certification stack

**Files:**
- Create: `deploy/pilot/compose.yaml`
- Create: `deploy/pilot/Caddyfile`
- Create: `deploy/pilot/README.md`
- Modify: `deploy/config/apheliondmm-collab.example.yaml`
- Modify: `deploy/runbooks/rollout.md`

- [ ] Add a disposable PostgreSQL, signed OIDC fixture, trusted TLS edge, and hosted service stack using non-secret example configuration.
- [ ] Validate configuration, health/readiness, OIDC login/logout, two-client editing, restart recovery, and clean shutdown.
- [ ] Scan the exact image and record image/configuration hashes.

### Task 6: Online human-test handoff

**Files:**
- Modify: `docs/testing/multiplayer-human-test-guide.md`
- Create: `docs/testing/multiplayer-online-pilot-guide.md`
- Create: `docs/verification/phase-7-desktop-hosted-pilot-2026-08-26.md`

- [ ] Run full repository, race, OpenAPI, container, database, OIDC, load, security-scan, and desktop build gates.
- [ ] Document operator-only deployment inputs: DNS name, TLS edge, PostgreSQL DSN secret source, OIDC issuer/client/redirect registration, trusted proxy CIDRs, and telemetry endpoint.
- [ ] Provide a simple named-tester guide covering sign-in, create/join, simultaneous edits, reconnect, conflicts, rename, safe reporting, and stop conditions.
