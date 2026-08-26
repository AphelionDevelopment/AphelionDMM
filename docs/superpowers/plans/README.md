# Multiplayer implementation roadmap

> **Current status (2026-08-26):** The active online architecture is the client-owned protocol-v2 relay in `2026-08-26-client-owned-relay.md`. The authoritative remaining-work ledger is `2026-08-26-final-multiplayer-progression-sheet.md`. The phases below are retained as protocol-v1 and shared-foundation implementation history. Content Tools remains excluded by explicit direction.

## Active protocol-v2 path

1. Deploy the locally verified stateless relay at `mapping.a13.info` without Cloudflare Access.
2. Run [`client-owned-relay-human-test-guide.md`](../../testing/client-owned-relay-human-test-guide.md) on two computers and separate networks.
3. Complete the deferred desktop owner-transfer flow before release acceptance.
4. Prepare a separate post-pilot cutover plan only after that evidence passes.

Execute these plans in order. Each phase is independently reviewable and has an explicit acceptance boundary.

| Phase | Plan | Produces | Entry condition |
| --- | --- | --- | --- |
| 1 | `2026-08-24-repository-foundation-and-reproducibility.md` | Pinned evidence, doctor command, baseline quality gates | Approved design |
| 2 | `2026-08-24-deterministic-operation-core.md` | Deterministic local operation engine and atomic saves | Phase 1 gates |
| 3 | `2026-08-24-local-collaboration-service.md` | Loopback authoritative service and two-client convergence | Phase 2 gates |
| 4 | `2026-08-24-collaboration-client-and-ux.md` | Desktop multiplayer, presence, conflicts, reconnect, undo | Phase 3 gates |
| 5 | `2026-08-24-collaboration-durability-and-security.md` | SQLite durability, recovery, security, telemetry | Phase 4 gates |
| 6 | `2026-08-24-aphelion-toolset-integration.md` | Versioned Meridian and Content Tools adapters | Phase 5 gates |
| 7 | `2026-08-24-hosted-rollout-and-operations.md` | PostgreSQL/OIDC hosted service and operational rollout | Phase 6 gates |

Do not skip the deterministic local core in order to demonstrate networking. Do not begin hosted rollout before restart recovery, authorization, and cross-repository acceptance are proven.

Every conditional commit step requires explicit user authorization. Without that authorization, leave verified changes in the working tree and report them as uncommitted.

No plan in this directory authorizes a commit or push. The current implementation remains uncommitted for review.
