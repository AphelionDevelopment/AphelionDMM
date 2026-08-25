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

## Verification

```text
go test ./internal/aphelion/collab/load ./cmd/apheliondmm-loadtest -count=1
go test -race ./internal/aphelion/collab/load -count=1
```

Both commands passed. The public-contract end-to-end test used three concurrent editor connections, six sequential durable operations, presence traffic, and final snapshot/hash convergence.

## Remaining acceptance

The full 25-editor recorded pilot has not yet been run against a reference hosted deployment. Service restart, database interruption, slow-consumer, and presence-flood injections remain outstanding, as do hosted p50/p95/p99 measurements and the zero-loss fault gate.
