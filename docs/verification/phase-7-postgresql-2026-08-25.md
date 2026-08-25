# Phase 7 PostgreSQL persistence verification

Date: 2026-08-25

## Dependency decision

`github.com/jackc/pgx/v5` v5.10.0 is pinned for the hosted store. It is MIT licensed and is the current stable v5 release reviewed during implementation. The release includes protocol-decoder, SCRAM, authentication-downgrade, and TLS cancellation hardening. It is newer than the patched releases for CVE-2024-27304, CVE-2026-33815, and CVE-2026-41889.

The initial module graph selected `golang.org/x/text` v0.37.0. `govulncheck` found reachable GO-2026-5970 through pgx pool configuration. The graph now pins v0.39.0, the first fixed release, and the repeated scan reports zero reachable vulnerabilities and zero vulnerabilities in imported packages.

Primary review sources:

- <https://github.com/jackc/pgx/blob/master/CHANGELOG.md>
- <https://github.com/jackc/pgx/security/advisories/GHSA-mrww-27vc-gghv>
- <https://github.com/advisories/GHSA-xgrm-4fwx-7qm8>
- <https://github.com/advisories/GHSA-j88v-2chj-qfwx>
- <https://pkg.go.dev/vuln/GO-2026-5970>

## Implementation

The PostgreSQL store uses the existing `store.SessionStore` contract. It adds:

- serializable create, append, and snapshot transactions;
- a per-document row lock and transactionally advanced current revision/hash;
- globally unique operation IDs and per-document contiguous revision keys;
- retained revision hashes and compactable snapshot-plus-replay recovery;
- a transaction-scoped advisory migration lock and future-schema rejection;
- bounded pgx pools, configured statement timeouts, context cancellation, and bounded serialization/deadlock retry;
- isolated optional schemas for conformance and hosted test fixtures.

## Runtime evidence

PostgreSQL 18.6-1 was installed user-locally through Scoop. Tests used loopback-only ephemeral ports and unique schemas; PostgreSQL was stopped after each run and was not registered as a Windows service.

The real PostgreSQL suite passed:

```text
go test ./internal/aphelion/collab/store/postgres -count=1 -v
```

Covered cases:

- shared store conformance;
- sixteen concurrent duplicate submissions split across two independent pools;
- context cancellation while another connection holds the document row lock, followed by a successful append;
- concurrent migration attempts;
- forced backend termination followed by pool recovery and a successful append;
- operation-ID collision across different documents without partial mutation;
- exact recovered snapshot, replay, and revision-hash parity with SQLite;
- retryable versus permanent PostgreSQL error classification.

The exact dependency graph scan passed:

```text
govulncheck ./...
```

Result: zero reachable vulnerabilities and zero vulnerabilities in imported packages. Nineteen findings exist only in required modules and are not called by this code.

This is local PostgreSQL integration evidence. It is not yet container, OIDC, rolling-upgrade, or hosted deployment acceptance; the logical restore rehearsal below is local fixture evidence rather than a deployed recovery exercise.

## Logical backup and restore rehearsal

The opt-in restore test uses PostgreSQL 18 `pg_dump` custom-format archives and `pg_restore` into a new empty database. Connection credentials are passed through the child environment rather than command arguments or test output. It reconstructs the document from the restored snapshot and replay, then compares the canonical hash with the stored revision hash.

Command:

```text
go test ./internal/aphelion/collab/store/postgres -run TestLogicalBackupRestoresExactRevisionAndHashBeforeAndAfterOperation -count=1 -v
```

Recorded local results:

- before the known second operation: 663.3833 ms, archive SHA-256 `5da603b7112416c70e296f34d680e72288a957d560b3994e1da177e1f60dd310`, revision 1, map hash `101b7d085526491cbab089dfade7b9ea1aa81937c3a22006d60e31ad091965b8`;
- after the known second operation: 506.1086 ms, archive SHA-256 `d4ecaf5adc3b74c613c08beaa718285f6548f2aaa303a8441108c7a98b9d317f`, revision 2, map hash `4a7127904f32e4a823b3ddf69ab744fc726d88b12aa5cc89a23f73d03c53daff`.

The temporary PostgreSQL server was bound to loopback and stopped cleanly after the test. These sub-second local measurements are evidence for the test fixture only, not a hosted RTO measurement. The pilot targets remain at most five minutes of unacknowledged backup exposure and 60 minutes for service restoration; acknowledged operations remain governed by transaction durability.
