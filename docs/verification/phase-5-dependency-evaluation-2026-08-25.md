# Phase 5 dependency evaluation

Date: 2026-08-25

This records evaluation and implementation evidence for the SQLite and OpenTelemetry dependencies proposed by `2026-08-24-collaboration-durability-and-security.md`. SQLite and OpenTelemetry are now selected and implemented. Observability acceptance evidence is recorded in `phase-5-observability-2026-08-25.md`.

## Local prerequisite setup

- Registered the approved Go 1.25.13 and golangci-lint 2.12.2 directories in the user `PATH`; retained the former linter as a versioned backup.
- Confirmed Rust 1.82.0 `x86_64-pc-windows-gnu` has Cargo, rustfmt, Clippy, rustc, rust-std, and rust-mingw installed.
- Confirmed Task 3.53.1 and GCC 15.2.0.
- Fetched the repository's locked Go modules and Rust crates.
- `go run ./cmd/apheliondmm-doctor` reports every declared prerequisite as `ok` through ordinary command resolution.
- `task verify` passes with `RUST_TARGET=1.82.0-x86_64-pc-windows-gnu`.

## Selected SQLite driver

`modernc.org/sqlite` v1.57.0 is selected:

- current stable module release at selection time;
- requires Go 1.25 and uses the approved Go 1.25.13 baseline;
- BSD-3-Clause, `database/sql`, and CGO-free;
- pins `modernc.org/libc` v1.74.4 as required by the driver documentation;
- returns SQLite 3.53.3 from `SELECT sqlite_version()`, above the 3.51.3 safety floor;
- includes the maintained super-journal corruption patch described by the upstream driver repository;
- passes the reusable store conformance suite on a real Windows database file;
- passes restart/reopen, concurrent duplicate append, future-schema rejection, locked-database rollback, injected append rollback, close, race, lint, and full Go tests;
- configures and verifies foreign keys, WAL, a 5-second busy timeout, full synchronous mode, and a single bounded connection.

The exact graph also raises `golang.org/x/sys` to v0.47.0 and `github.com/mattn/go-isatty` to v0.0.24 through minimal version selection. `govulncheck ./...` reports zero reachable vulnerabilities and zero vulnerabilities in imported packages with this graph.

References:

- <https://pkg.go.dev/modernc.org/sqlite@v1.57.0>
- <https://gitlab.com/cznic/sqlite/-/blob/master/CHANGELOG.md>
- <https://sqlite.org/releaselog/3_51_3.html>

## Retained fallback evidence

`github.com/mattn/go-sqlite3` v1.14.50 remains a viable MIT-licensed CGO fallback. Its isolated Windows probe returned SQLite 3.53.4 and produced a static executable. It was not selected because the pure-Go driver reduces platform-specific collaboration-service linking and deployment risk.

`github.com/ncruces/go-sqlite3` remained pre-v1 at evaluation time and was not selected.

## Selected OpenTelemetry dependency

`go.opentelemetry.io/otel`, its trace SDK, and its metric SDK v1.45.0 are selected after the protected Go 1.25.13 upgrade. They require Go 1.25, are Apache-2.0 licensed, and passed the exact-graph vulnerability scan. Providers are injected explicitly and exporters remain optional. The initial implementation uses stable traces and metrics plus existing structured logs; it does not adopt the beta logs signal.

## Security prerequisite

The exact pinned graph was scanned with `govulncheck` v1.7.0:

- Go 1.24.0 produced 32 reachable findings: 31 in the standard library and one in `golang.org/x/image` v0.28.0.
- Running the unchanged repository with Go 1.25.13 passed `go test ./... -count=1` and removed all reachable standard-library findings.
- The remaining `GO-2026-5031` finding reaches the BMP decoder through `internal/rsc/png.go`; its fixed `golang.org/x/image` v0.41.0 release requires Go 1.25.
- golangci-lint 2.1.5 was built with Go 1.24 and cannot analyze a Go 1.25 module. Official Go 1.25 support begins at golangci-lint 2.4.0; the protected proposal selects the verified 2.12.2 release.

The exact protected change and successful acceptance results are recorded in `toolchain-security-audit-2026-08-25.md`. The approved repository baseline is now Go 1.25.13, golangci-lint 2.12.2, and `golang.org/x/image` v0.41.0; Phase 5 dependency selection must use that baseline.

References:

- <https://opentelemetry.io/docs/languages/go/>
- <https://pkg.go.dev/go.opentelemetry.io/otel@v1.41.0>
- <https://opentelemetry.io/docs/languages/go/exporters/>

## Required implementation gates

- Add the SQLite driver only with the Task 2 failing conformance test already present.
- Assert the runtime SQLite version is at least 3.51.3.
- Run on-disk WAL, duplicate-append, lock-timeout, migration, close, race, static-build, and restart tests.
- Run `govulncheck` and dependency-license review against the exact proposed module graph before acceptance.
- Keep exporter configuration out of collaboration messages and make zero-value telemetry network-free.
