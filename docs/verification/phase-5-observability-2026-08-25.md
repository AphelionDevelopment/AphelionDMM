# Phase 5 observability verification

Date: 2026-08-25

This records acceptance evidence for the OpenTelemetry observability and readiness work in Task 5 of `2026-08-24-collaboration-durability-and-security.md`.

## Dependency decision

OpenTelemetry Go API, trace SDK, and metric SDK v1.45.0 are pinned as direct dependencies. The selected release requires Go 1.25 and therefore matches the approved Go 1.25.13 repository baseline. The modules are Apache-2.0 licensed.

Providers are injected through collaboration configuration. A service with no telemetry value does not initialize an exporter, make an outbound connection, or enter the instrumentation path. Constructing the telemetry facade without explicit providers uses the OpenTelemetry no-op global providers and still configures no exporter.

## Implemented signals

- operation spans and counters distinguish submit and inverse operations and record only success or error outcomes;
- store spans and counters cover create, append, snapshot, and load operations;
- replay spans and counters cover recovery replay and reconnect replay;
- a presence-drop counter records coalesced subscriber updates;
- a connection up/down counter records accepted WebSocket lifetimes;
- attributes contain bounded operation categories and outcomes, not tokens, map content, document identifiers, actor identifiers, or raw error messages.

Liveness remains process-only. Readiness reports unavailable when a document cannot be recovered and returns to ready after recoverable ownership is established.

## Focused evidence

- telemetry and server packages passed ten consecutive runs;
- in-memory trace and metric exporters observed operation, store, replay, presence-drop, and connection signals;
- the sensitive-error fixture was absent from every exported span attribute;
- recovery, reconnect replay, snapshots, presence coalescing, submit, and inverse paths are instrumented through injected providers;
- the disabled telemetry benchmark recorded `0 B/op` and `0 allocs/op` in three runs;
- the no-exporter facade benchmark recorded approximately `768 B/op` and `12 allocs/op`; this path is opt-in and configures no network exporter.

Benchmark command:

```powershell
go test ./internal/aphelion/collab/telemetry -run '^$' -bench 'Benchmark(Disabled|NoExporter)Telemetry$' -benchmem -count=3
```

## Security and repository gates

- `go mod tidy` and `go mod verify`: pass;
- `govulncheck ./...`: 0 reachable vulnerabilities and 0 vulnerabilities in imported packages; 19 module findings are unreachable from this code;
- `go test ./... -count=1`: pass;
- `go test -race ./internal/aphelion/... -count=1`: pass;
- golangci-lint 2.12.2: `0 issues.`;
- `task verify` with Go 1.25.13 and `RUST_TARGET=1.82.0-x86_64-pc-windows-gnu`: pass, including Go tests, locked Rust tests, rustfmt, Clippy with warnings denied, release parser build, and the static Windows desktop build;
- `git diff --check`: pass with line-ending conversion warnings only.

The inherited ImGui/GCC `memset` warning remains unchanged. No exporter or collector deployment was added, and no public network was used by collaboration tests.
