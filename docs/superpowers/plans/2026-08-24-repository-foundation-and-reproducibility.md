# Repository Foundation and Reproducibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish trustworthy, machine-readable toolchain evidence and quality gates before multiplayer behavior is introduced.

**Architecture:** Add an Aphelion-owned doctor library and command without replacing the existing Task entry points. Pin the Rust selection in a new repository file, then strengthen protected build/CI configuration only after exact-file user approval.

**Tech Stack:** Go 1.24, Rust 1.82 GNU targets, PowerShell, Task 3, golangci-lint 2.1.5, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-08-24-multiplayer-design.md`

## Global Constraints

- Read `AGENTS.md`, `docs/agent/source-authority.md`, and `docs/agent/verification.md` first.
- Use PowerShell and inspect `$LASTEXITCODE` after native commands.
- Do not change `.github/workflows/ci.yml`, `Taskfile.yml`, `Taskfile_windows.yml`, release, signing, or updater publication without explicit user confirmation naming the file and effect.
- Do not commit unless the user explicitly authorizes commits; each commit step below is conditional.
- Preserve StrongDMM naming and assets during this phase.

---

### Task 1: Add machine-readable toolchain expectations

**Files:**
- Create: `internal/aphelion/buildcheck/manifest.go`
- Create: `internal/aphelion/buildcheck/manifest_test.go`
- Create: `tools/toolchain/manifest.json`
- Create after protected-infrastructure approval: `rust-toolchain.toml`

- [ ] Write `manifest_test.go` with a fixture asserting strict decode, required tools, and rejection of unknown JSON fields.

```go
func TestDecodeManifest(t *testing.T) {
	manifest, err := Decode(strings.NewReader(`{
		"go":"1.24.0",
		"rust":"1.82.0",
		"rust_target":"x86_64-pc-windows-gnu",
		"task_major":3,
		"golangci_lint":"2.1.5"
	}`))
	require.NoError(t, err)
	assert.Equal(t, "1.82.0", manifest.Rust)
}
```

- [ ] Run `go test ./internal/aphelion/buildcheck -run TestDecodeManifest -count=1` and confirm it fails because the package is absent.
- [ ] Implement the strict decoder.

```go
type Manifest struct {
	Go           string `json:"go"`
	Rust         string `json:"rust"`
	RustTarget   string `json:"rust_target"`
	TaskMajor    int    `json:"task_major"`
	GolangCILint string `json:"golangci_lint"`
}

func Decode(reader io.Reader) (Manifest, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode toolchain manifest: %w", err)
	}
	if manifest.Go == "" || manifest.Rust == "" || manifest.RustTarget == "" || manifest.TaskMajor < 1 {
		return Manifest{}, errors.New("toolchain manifest has missing required values")
	}
	return manifest, nil
}
```

- [ ] Add `tools/toolchain/manifest.json` with Go `1.24.0`, Rust `1.82.0`, Windows GNU target `x86_64-pc-windows-gnu`, Task major `3`, and golangci-lint `2.1.5`.
- [ ] Obtain explicit approval, then add `rust-toolchain.toml` selecting channel `1.82`, profile `minimal`, and components `rustfmt` and `clippy`. Do not modify `third_party/sdmmparser` sources.
- [ ] Re-run the focused test and confirm it passes.
- [ ] If authorized, commit with `build: record reproducible toolchain expectations`; otherwise record the verified uncommitted files.

### Task 2: Add a non-mutating environment doctor

**Files:**
- Create: `internal/aphelion/buildcheck/check.go`
- Create: `internal/aphelion/buildcheck/check_test.go`
- Create: `cmd/apheliondmm-doctor/main.go`

- [ ] Write table tests for exact Go/Rust versions, Task major compatibility, missing tools, and unexpected command output.

```go
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type Result struct {
	Name     string
	Expected string
	Actual   string
	OK       bool
	Detail   string
}

func Check(ctx context.Context, runner Runner, manifest Manifest) []Result
```

- [ ] Run `go test ./internal/aphelion/buildcheck -run TestCheck -count=1` and confirm failure for the missing implementation.
- [ ] Implement fixed command probes for `go version`, `rustup run 1.82-x86_64-pc-windows-gnu rustc --version`, `task --version`, `golangci-lint version`, and `gcc --version`. Do not accept command names or arguments from user/network input.
- [ ] Make `cmd/apheliondmm-doctor` read only the repository manifest, emit one line per tool, and return exit code 1 when required checks fail.
- [ ] Run the focused tests.
- [ ] Run `go run ./cmd/apheliondmm-doctor` and preserve the exact environment report even when tools are missing.
- [ ] If authorized, commit with `build: add AphelionDMM environment doctor`.

### Task 3: Establish baseline Go and Rust test gates

**Files:**
- Create: `internal/aphelion/buildcheck/gates_test.go`
- Modify after approval: `Taskfile.yml`
- Modify after approval: `.github/workflows/ci.yml`

- [ ] Add a test that parses `Taskfile.yml` and asserts named `test-go`, `test-rust`, and `verify` tasks after the approved change.
- [ ] Run `go test ./internal/aphelion/buildcheck -run TestTaskfileDefinesVerificationGates -count=1` and confirm it fails.
- [ ] Present the exact Taskfile additions and CI job effects to the user and obtain explicit protected-infrastructure approval.
- [ ] Add Task targets that run:

```yaml
test-go:
  cmds:
    - go test ./... -count=1

test-rust:
  dir: third_party/sdmmparser/src
  cmds:
    - RUSTUP_TOOLCHAIN={{.RUST_TARGET}} cargo test --locked
    - RUSTUP_TOOLCHAIN={{.RUST_TARGET}} cargo fmt --all -- --check
    - RUSTUP_TOOLCHAIN={{.RUST_TARGET}} cargo clippy --all-targets --locked -- -D warnings

verify:
  cmds:
    - task: test-go
    - task: test-rust
    - task: build
```

- [ ] Add CI test jobs using Go from `go.mod`, explicit Rust 1.82 target-qualified toolchains, and existing platform native dependencies. Keep release behavior unchanged.
- [ ] Run the Taskfile contract test, then `task verify` with the exact Windows `RUST_TARGET`.
- [ ] If authorized, commit with `ci: run repository tests and pinned verification`.

### Task 4: Restore high-signal static analysis deliberately

**Files:**
- Modify after approval: `.golangci.yml`
- Modify: files identified by the newly enabled diagnostics
- Create: `docs/verification/static-analysis-baseline.md`

- [ ] Record current disabled linters and run golangci-lint 2.1.5 without changing configuration.
- [ ] Obtain exact-file approval for `.golangci.yml`.
- [ ] Enable `errcheck`, `govet`, and `staticcheck` one at a time. For each, capture the diagnostic count before editing code.
- [ ] Fix diagnostics with behavior-preserving changes and narrow `APHELION EDIT` markers in inherited files. Do not add blanket exclusions.
- [ ] Add `docs/verification/static-analysis-baseline.md` listing command, version, date, resolved diagnostics, and any line-specific exclusion with rationale.
- [ ] Run `golangci-lint run`, `go test ./... -count=1`, and `task build`.
- [ ] If authorized, commit with `quality: restore core Go analyzers`.

### Task 5: Add real-entry-point smoke evidence

**Files:**
- Create: `internal/aphelion/smoke/report.go`
- Create: `internal/aphelion/smoke/report_test.go`
- Create: `cmd/apheliondmm-smoke/main.go`
- Create: `testdata/smoke/minimal.dme`
- Create: `testdata/smoke/minimal.dmm`

- [ ] Write tests for a report that records executable revision, fixture hashes, open result, save result, and clean shutdown.
- [ ] Run `go test ./internal/aphelion/smoke -count=1` and confirm it fails.
- [ ] Implement a report writer and a smoke command that orchestrates only trusted local test fixtures. Keep UI automation behind an interface so Windows entry-point automation can be added without altering domain checks.
- [ ] Add a minimal human-authored neutral fixture containing no new lore, descriptions, or branding.
- [ ] Run `go test ./internal/aphelion/smoke -count=1`.
- [ ] Build with `task build`, run `dst/StrongDMM.exe` through the approved smoke path, and save the report under the temporary test output directory rather than the repository.
- [ ] If authorized, commit with `test: add desktop entry-point smoke reporting`.

## Phase acceptance

- [ ] Environment doctor reports exact installed and missing prerequisites.
- [ ] Repository Go/Rust test and build gates pass with pinned versions, or every unavailable gate is recorded without a completion claim.
- [ ] Static analyzers are enabled without blanket suppression.
- [ ] The shipped desktop entry point has an evidence-producing smoke path.
- [ ] No branding or release behavior changed.

