# Phase 5 resilience verification

Date: 2026-08-25

This records Task 7 fault, corruption, saturation, and CI evidence.

## Covered failure boundaries

- failed store append leaves the authoritative revision unchanged and the SQLite transaction reusable;
- interrupted snapshot writes report failure while the acknowledged operation remains recoverable from the log;
- discontinuous replay and corrupt snapshots fail closed;
- a deliberately corrupted copy of a real SQLite database cannot be loaded, while the untouched source reopens with its acknowledged operation;
- presence pressure is coalesced and cannot block durable delivery;
- operation, connection, request, and queue limits bound saturation behavior;
- service shutdown and restart tests cover token cleanup, persisted replay, and clean ownership closure.

## CI durability gate

The approved `.github/workflows/ci.yml` now includes a separate `collaboration-resilience` Ubuntu job using the Go version from `go.mod`. It runs:

- `go test -race ./internal/aphelion/... -count=1`;
- focused SQLite restart, rollback, and copied-corruption tests;
- focused interrupted-snapshot, discontinuous-replay, and presence-pressure tests;
- five-second client and server protocol fuzz smoke gates.

The existing build and release jobs are unchanged apart from the previously approved toolchain pin update. The new resilience job does not publish or sign artifacts.

## Local evidence

- SQLite and server fault selections passed ten consecutive runs;
- client envelope fuzz: approximately 394,254 executions, pass;
- server envelope fuzz: approximately 325,354 executions, pass;
- the real `go run ./cmd/apheliondmm-smoke` entry point completed parser round-trip and collaboration smoke with `status=ok`;
- the final full package test passed after the Task 7 additions;
- `git diff --check` passed with line-ending conversion warnings only.

The final pre-Task-7 `task verify` also passed Go tests, locked Rust tests, rustfmt, Clippy with warnings denied, release parser build, and the static Windows desktop build. Task 7 changed only tests and the approved CI workflow after that cross-stack build.
