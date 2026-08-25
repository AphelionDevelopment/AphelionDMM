# Toolchain security audit and protected change proposal

Date: 2026-08-25

This audit records evidence against the former repository pins and the verified result of the protected build and CI change approved on 2026-08-25.

## Current evidence

- The declared Go baseline is 1.24.0 in `go.mod` and `tools/toolchain/manifest.json`.
- `govulncheck` v1.7.0 reports 32 reachable vulnerabilities under Go 1.24.0: 31 standard-library findings and `GO-2026-5031` in `golang.org/x/image` v0.28.0.
- The image finding is reachable through `internal/rsc/png.go` and is fixed in `golang.org/x/image` v0.41.0, which requires Go 1.25.
- The unchanged repository passes `go test ./... -count=1` with Go 1.25.13.
- Re-scanning with Go 1.25.13 removes all reachable standard-library findings, leaving only the image-module finding.
- golangci-lint 2.1.5 was built with Go 1.24.0. golangci-lint supports target Go versions no newer than its build toolchain; official Go 1.25 support starts at v2.4.0.
- The official golangci-lint 2.12.2 Windows archive was installed in the candidate cache after its published SHA-256 checksum matched. It was built with Go 1.26.2, accepts the repository configuration, and can analyze Go 1.25 code.
- Running golangci-lint 2.12.2 with Go 1.25.13 exposes two new `QF1012` findings in `internal/dmapi/dmmap/dmmdata/save_dm.go` and `save_tgm.go`; the protected lint-pin update must include those focused source remediations or CI will fail.
- CI uses `actions/setup-go@v6` with `go-version-file: go.mod`, so editing the Go directive changes the CI compiler without requiring a workflow source edit.

## Approved and applied protected change set

1. Change the `go.mod` Go directive from 1.24.0 to 1.25.13.
2. Change `tools/toolchain/manifest.json` to require Go 1.25.13.
3. Upgrade `golang.org/x/image` from v0.28.0 to v0.41.0 and accept the minimal-module-selection changes produced by `go mod tidy`.
4. Upgrade the golangci-lint pin from 2.1.5 to 2.12.2, update the manifest and CI action input, and update doctor/buildcheck fixtures.
5. Remediate the two `QF1012` findings in `internal/dmapi/dmmap/dmmdata/save_dm.go` and `save_tgm.go` by replacing `WriteString(fmt.Sprintf(...))` with `fmt.Fprintf(...)`.
6. Update current verification documentation and version expectations. Do not rewrite historical evidence as if it used the new toolchain.

No Taskfile source was changed. No `.github/workflows/ci.yml` structural change was made beyond the explicit golangci-lint version input; its Go setup already follows `go.mod`.

## Acceptance results

- The repository doctor reports Go 1.25.13, Rust 1.82.0, Task 3.53.1, golangci-lint 2.12.2, and GCC 15.2.0 as `ok`.
- `go mod tidy` changed only the intended `golang.org/x/image` sums.
- `go test ./... -count=1` passes.
- `go test -race ./internal/aphelion/... -count=1` passes.
- `govulncheck ./...` reports zero reachable vulnerabilities.
- golangci-lint 2.12.2 reports `0 issues.`
- `task verify` passes with the locked Rust 1.82 GNU target, including the static Windows desktop build.
- The produced `apheliondmm-smoke` executable passes parser round-trip plus a real two-client collaboration session, revisions 1 and 2, identical final hashes, leave, and shutdown.
- The inherited ImGui/GCC `memset` warning remains unchanged.
- The existing protocol and trust-boundary behavior was not altered by the toolchain change.

The active user `PATH` now selects Go 1.25.13 and golangci-lint 2.12.2. The former golangci-lint 2.1.5 executable is retained as `golangci-lint-2.1.5.exe` in the local toolchain bin directory.

## Sources

- Go vulnerability database and scanner: <https://go.dev/security/vuln/>
- golangci-lint Go-version support: <https://golangci-lint.run/docs/welcome/faq/>
- golangci-lint Go 1.25 support record: <https://github.com/golangci/golangci-lint/issues/5873>
