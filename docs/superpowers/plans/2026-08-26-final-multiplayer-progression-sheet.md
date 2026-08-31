# AphelionDMM Multiplayer Final Progression Sheet

> **Superseded on 2026-08-31:** Current decisions are in `../../verification/multiplayer-implementation-readiness.md`, `../../verification/online-pilot-readiness.md`, and `../../verification/public-hosting-readiness.md`. This sheet is historical and must not be used to infer fresh external-service, public-hosting, or human evidence.

**Status date:** 2026-08-26  
**Scope:** AphelionDMM collaboration, Meridian integration contracts, local hosted service, and preparation for a private online pilot.  
**Excluded:** aphelion-content-tools work, public discovery, and multi-replica hosting.

## Bottom line

AphelionDMM is ready for human desktop acceptance testing. The planned single-replica collaboration implementation and its local automated verification are complete. No known missing core feature blocks a two-user test.

The project is not yet ready for an unattended Internet-facing production rollout. That later milestone still requires external identity and infrastructure configuration, a controlled load/fault exercise, backup and rollback rehearsal, production telemetry, and release signing ownership.

## Milestone progression

| Milestone | State | What remains |
| --- | --- | --- |
| Repository and reproducible toolchain | Complete | Four inherited `go vet` `unsafe.Pointer` diagnostics remain outside collaboration code. The configured lint gate reports no issues. |
| Deterministic operation core | Complete | Human regression of ordinary single-user edit, undo, redo, save, close, and reopen. |
| Local collaboration service | Complete | Real two-window desktop acceptance. |
| Collaboration desktop UX | Implementation complete | Human visual, keyboard, narrow-layout, reconnect, conflict, presence, close/reopen, and status-without-color checks. |
| Durability and security | Local automated scope complete | Human process/interruption behavior and production updater-signing ownership. |
| AphelionDMM and Meridian contracts | Complete | Cross-repository human acceptance when Meridian integration is exercised. Content Tools remains explicitly excluded. |
| Hosted single-replica service | Portable deployment locally verified | Server inventory, external OIDC, dedicated public tunnel/DNS, hosted load/fault evidence, production telemetry, and named-user pilot. |
| Replay and owner-identity repair | Implementation complete | Human two-window confirmation under reconnect/desynchronization pressure and owner rename verification. |
| Desktop hosted sign-in and invite flow | Implementation complete | Human two-client hosted editing and restart/recovery test against the selected deployment. |

## What is complete

- Deterministic shared operation model, validation, sequencing, conflict behavior, undo/redo handling, replay, and recovery.
- Local collaboration lifecycle: start, invite, join, leave, reconnect, close, and session ownership behavior.
- Owner display-name editing and participant identity propagation.
- Desktop collaboration panel, presence, selection/cursor transport, participant display, connection state, and conflict/status presentation.
- Persistence, database migrations, token handling, invitation security boundaries, rate and payload controls, telemetry surfaces, and updater fail-closed behavior.
- Single-replica hosted service, PostgreSQL-backed deployment, reverse proxy/TLS pilot stack, OIDC browser sign-in flow, desktop hosted-session lifecycle, and operator documentation.
- Replay-repair regression coverage and collaboration package race testing.
- Windows StrongDMM release build and local Docker pilot validation.
- OpenAPI and AsyncAPI validation, dependency and container vulnerability checks, and container lifecycle verification.
- Human test guide and hosted-pilot operating documentation.

## Remaining human acceptance gate

Run two real StrongDMM windows and record any failure with both clients' logs and the exact action sequence.

1. Open the same representative map in both windows.
2. Start a local session, copy the invite, join from the second window, and change both participant names, including the owner.
3. Make simultaneous edits in different locations, then overlapping edits to exercise conflict presentation.
4. Exercise undo and redo from both clients.
5. Interrupt one client, continue editing from the other, reconnect, and confirm convergence without duplicated or missing edits.
6. Verify remote cursor and selection z-level behavior and expiry, and confirm that presence never changes the saved map.
7. Save, close, reopen, and reparse the map; compare the reopened result between clients.
8. Repeat ordinary single-user editing before and after collaboration.
9. Check keyboard access, narrow layout, readable state without color, focus order, and screen-reader announcements where available.
10. Repeat the essential workflow against the hosted pilot after its external endpoint and OIDC provider are configured.

## Remaining online-pilot work

These are deployment and operational gates, not missing editor features.

The game-server executor should begin with [`docs/hosting/game-server-deployment-agent-handoff.md`](../../hosting/game-server-deployment-agent-handoff.md). It contains the server inventory, immutable-source, OIDC, dedicated-tunnel, rollout, rollback, evidence, and stop-condition checklist.

- Complete the interactive Cloudflare Access login and inventory the Meridian server's Docker capacity, storage, service account, and backup destination.
- Install the locally verified Compose project on the Meridian server with ACL-restricted secrets and PostgreSQL storage.
- Create the dedicated public tunnel and publish `mapping.a13.info` without a Cloudflare Access application after local readiness succeeds.
- Connect and certify the selected external OIDC provider, including logout, token expiry, revocation, and named-user access.
- Deploy exactly one service replica. Multi-replica operation remains unsupported until cross-instance event fanout is implemented.
- Run the 25-editor profile against the reference hosted deployment and record p50, p95, and p99 latency, reconnect counts, conflicts, and storage health.
- Inject slow consumers, sustained presence traffic, and a database interruption; confirm final zero-loss convergence.
- Repeat the already verified backup restore, service stop/start, and rollback workflow on the reference server using the exact deployed artifacts.
- Configure dashboards and alerts for service health, connection churn, operation latency, database health, and authentication failures.
- Assign the production minisign key owner and publish signed updater metadata before enabling updater downloads.
- Conduct a private named-user pilot before considering a wider rollout.

## Residual engineering notes

These do not block the first human test, but they should remain visible:

- Hosted invitation decoding now rejects expired or missing expiry data before redemption. Server redemption remains authoritative.
- Hosted service-origin validation and canceled-sign-in credential handling now have focused client coverage. Dear ImGui menu rendering and browser usability remain human acceptance work because the inherited application shell has no headless GUI harness.
- The PostgreSQL readiness refusal was traced to the image's temporary Unix-socket initialization server. Compose and the container harness now wait for the final TCP listener, with static and real lifecycle coverage.
- Raw `go vet ./...` still reports four inherited OpenGL `unsafe.Pointer` diagnostics outside the collaboration implementation. They are not a multiplayer regression, but they remain repository technical debt.
- All work remains uncommitted and unpushed unless separately authorized. Conditional commit checkboxes in older plans are therefore not incomplete engineering work.

## Earlier-plan reconciliation

Unchecked boxes in the early foundation, deterministic-core, local-service, durability, replay-repair, and desktop-hosted plans largely record historical test-driven implementation steps or conditional commit commands. Their later status sections and the consolidated readiness ledger supersede them; they are not an active backlog.

The active backlog is limited to:

1. Human desktop acceptance.
2. Meridian server inventory, external OIDC registration, and public `mapping.a13.info` deployment.
3. Hosted load, fault, recovery, telemetry, and reference-server rollback evidence.
4. Production updater signing ownership.
5. Inherited repository technical debt listed above.

Content Tools stays deferred to the other agent. Multi-replica hosting and public session discovery stay intentionally out of scope.

## Readiness decision

| Target | Decision |
| --- | --- |
| Begin human local testing | **Ready now** |
| Begin human testing against the local Docker-hosted stack | **Ready now** |
| Start a private Internet pilot | **Pending external infrastructure and operator gates** |
| Enable production updater downloads | **Blocked on minisign ownership and signed publication** |
| Run multiple service replicas | **Not supported by the present plan** |
| Claim general production completion | **Not yet justified** |
