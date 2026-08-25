# Phase 5 updater security verification

Date: 2026-08-25

This records acceptance evidence for Task 6 of `2026-08-24-collaboration-durability-and-security.md`.

## Protected change scope

The approved protected changes are limited to:

- `internal/req/req.go`: replace the default unbounded HTTP path with an injectable client, connect/TLS/header/total deadlines, response byte limits, status validation, and optional media-type validation;
- `internal/app/selfupdate/manifest.go`: require a bounded, strict JSON envelope whose manifest payload has a valid minisign signature and whose platform artifacts include HTTPS URLs, lowercase SHA-256 digests, and minisign signatures;
- `internal/app/selfupdate/selfupdate.go`: verify artifact SHA-256 and minisign signature before invoking the existing `minio/selfupdate` replacement mechanism, retain the previous executable, and surface rollback failure distinctly;
- `internal/app/update.go`: select the signed platform artifact, download it through the bounded client, and pass its verification metadata into replacement.

No CI, Taskfile, publication, signing-secret, or release workflow source was changed. Inherited StrongDMM source is retained in `APHELION EDIT` removal markers.

## Trust-root decision

`TrustedPublicKey` is a linker-injected minisign public key. Its default is empty and therefore fail-closed: update discovery returns `update signing key is not configured` before any network request. The current unsigned legacy StrongDMM manifest is not trusted.

Production self-update remains intentionally disabled until the human-owned Aphelion release pipeline supplies an approved public key, signed manifest envelope, and signed artifacts. Private signing material must never enter this repository or client configuration.

## Offline test evidence

All request/update behavior is tested with local transports, `httptest.Server`, `httptest.NewTLSServer`, and temporary executable files. Tests cover:

- bounded connect, TLS handshake, response-header, and total/body time;
- non-success HTTP status;
- unexpected media type;
- declared and streamed response-size overflow;
- invalid manifest signature;
- invalid artifact signature;
- truncated artifact/hash mismatch;
- replacement preparation failure preserving the installed bytes;
- successful verified replacement retaining the old executable at the configured `.old` path.

## Verification

- request and updater tests, ten consecutive runs: pass;
- updater/request race tests: pass;
- `go mod tidy` and `go mod verify`: pass;
- `go test ./... -count=1`: pass;
- `govulncheck ./...`: 0 reachable vulnerabilities and 0 vulnerabilities in imported packages; 19 module findings are unreachable;
- golangci-lint 2.12.2: `0 issues.`;
- `task verify` with Go 1.25.13 and `RUST_TARGET=1.82.0-x86_64-pc-windows-gnu`: pass, including locked Rust tests, rustfmt, Clippy with warnings denied, release parser build, and static Windows desktop build;
- `git diff --check`: pass with line-ending conversion warnings only.

The inherited ImGui/GCC `memset` warning remains unchanged.
