# Protocol compatibility matrix

Status date: 2026-08-31

| Contract | Supported value | Evidence | Limit |
| --- | --- | --- | --- |
| Collaboration protocol | 1 | Golden client/server envelopes, semantic OpenAPI/AsyncAPI tests, two-client smoke | No cross-version rolling pair declared |
| Snapshot schema | 1 | Canonical golden hash, bounded dimension/cell tests, replay/recovery tests | No schema-2 migration exists |
| Integration manifest schema | 1 | Strict decoder, canonical hash, shipped Meridian command test | Real Meridian cross-stack acceptance unrun |
| Export checkpoint | Pending, accepted, rejected states | Model/store conformance, SQLite restart, owner/precondition/idempotency HTTP tests, semantic OpenAPI test | Live PostgreSQL completion unrun |

Compatibility is gated before join. Unknown protocol/schema versions and changed-semantics fixtures fail closed. Previous/current rolling service pairs and multi-replica operation are not declared supported and have not been tested.
