# Phase 7 deterministic load tooling evidence

Date: 2026-08-25

## Implemented

- Scenario generation is deterministic from a recorded seed and produces bounded actor, durable-operation, and presence streams.
- Operations use neutral structural turf paths and do not add authored map content.
- Every generated scenario records and self-verifies its expected final revision and canonical map hash.
- The checked-in pilot profile specifies 25 concurrent editors, 250 operations paced at 10 per second, 20 presence updates per second per editor, and a 250 ms p95 acknowledgement ceiling.
- The runner uses only the public join-token HTTP endpoint, collaboration WebSocket subprotocol, and snapshot HTTP endpoint.
- Each durable operation must be observed by every connected client before the next operation begins. The final authoritative snapshot must match the expected revision and hash.
- The command reads the owner credential only from `APHELIONDMM_LOAD_OWNER_TOKEN` and emits JSON p50/p95/p99 latency, counts, convergence state, and gate outcome.
- Hosted runs additionally read one OIDC session credential per simulated editor from the bounded, permission-checked JSON file named by `APHELIONDMM_LOAD_EDITOR_TOKENS_FILE`. The runner creates and redeems ordinary hosted invitations and never calls the hosted-forbidden embedded join-token endpoint. Tokens remain memory-only and are excluded from results and bounded errors.

## Verification

```text
go test ./internal/aphelion/collab/load ./cmd/apheliondmm-loadtest -count=1
go test -race ./internal/aphelion/collab/load -count=1
```

Both commands passed. The public-contract end-to-end test used three concurrent editor connections, six sequential durable operations, presence traffic, and final snapshot/hash convergence.

The recorded 25-editor loopback pilot also passed after the hosted-container work:

```text
clients=25 operations=250 presence=500 p50=3.00ms p95=9.03ms p99=23.80ms revision=250
hash=71913e2be13a2206141dd93bc0d4243eef7a47f33a92b7425ce399966189f05f
```

This is deterministic same-host public-contract evidence, not the reference hosted deployment measurement.

## Remaining acceptance

The full 25-editor recorded pilot has not yet been run against a reference hosted deployment. The OCI lifecycle gate now covers graceful service restart, durable snapshot recovery, database interruption, and readiness recovery. Slow-consumer and presence-flood injection during the reference load run remain outstanding, as do hosted p50/p95/p99 measurements and the zero-loss final-convergence fault gate.
