# AphelionDMM Code Audit

**Audit date:** 2026-08-30  
**Revision:** `c3469c60` (`main`, matching `origin/main`)  
**Baseline:** `5241698a` (the approved multiplayer design's reviewed StrongDMM revision)  
**Working tree:** Clean before and after the audit  
**Remediation plan:** [`../superpowers/plans/2026-08-30-admm-audit-remediation.md`](../superpowers/plans/2026-08-30-admm-audit-remediation.md)

## Decision

The local and two-client collaboration foundations are substantial and currently pass repository, race, Rust, build, doctor, smoke, and vulnerability gates. The checkout is suitable for continued local human testing.

The repository is not ready for a production or cross-stack completion claim. One hostile hosted snapshot can panic or exhaust a joining desktop because map dimensions are not bounded and their product is multiplied without overflow checks. Durable append error handling can also leave the store one revision ahead of the in-memory document after an ambiguous post-commit error. The advertised export-checkpoint API is a hard-coded `501`, and the Meridian staging and acceptance abstractions cannot be composed in their normal configuration.

Current readiness documents therefore overstate the implemented surface. Their human and external-infrastructure gates remain valid, but they are not the only remaining work.

## Remediation update — 2026-08-31

All eight audit findings are repaired in the uncommitted working tree and pass their focused gates. Fresh full local qualification also passed after correcting a snapshot-failure hot-retry uncovered by the first repository rerun.

| Finding | Current state | Fresh evidence |
| --- | --- | --- |
| ADMM-AUDIT-001 | Fixed | Model/adapter/protocol/hosted dimension limits and full repository/race tests passed |
| ADMM-AUDIT-002 | Fixed | Committed-then-error, durable-ahead duplicate, and conflicting-ID tests passed across server/store gates |
| ADMM-AUDIT-003 | Fixed locally | Owner-authorized durable checkpoint route, store conformance, SQLite restart/completion, and semantic OpenAPI tests passed; live PostgreSQL unrun |
| ADMM-AUDIT-004 | Fixed locally | Immutable-artifact verifier and shipped `apheliondmm-meridian-verify` command/race tests passed; real Meridian-MCP/Rift/Content Tools acceptance unrun because no installed `meridian-mcp.exe` was available |
| ADMM-AUDIT-005 | Fixed locally | `golangci-lint run`, release dependency policy, immutable action policy, `actionlint`, and expanded `task verify` passed; hosted GitHub Actions unrun |
| ADMM-AUDIT-006 | Fixed | Windows broad-read DACL rejection, restricted-DACL acceptance, server tests, and race tests passed |
| ADMM-AUDIT-007 | Fixed | Model dependency policy, standard-library UUIDv7 tests, affected package tests, and ownership-marker review passed; fresh upstream fetch remained at `5241698a` |
| ADMM-AUDIT-008 | Fixed locally | Semantic contract gates and full local aggregate passed; raw inherited `go vet` diagnostics remain separately documented |

Fresh evidence and remaining gates are authoritative in `multiplayer-implementation-readiness.md`, `online-pilot-readiness.md`, `public-hosting-readiness.md`, and `protocol-compatibility-matrix.md`. Production, public hosting, and real Meridian integration are not complete claims.

## Scope and accounting

The audit covered repository guidance, approved designs, code added since `5241698a`, ownership boundaries, desktop adapters, operation and persistence layers, HTTP/WebSocket/OIDC surfaces, deployment and CI, updater security, Meridian integration, contracts, tests, and local verification entry points.

Current repository accounting:

| Measure | Current checkout |
| --- | ---: |
| Tracked files | 540 |
| Go files | 388 |
| Go test files | 86 |
| Go test/fuzz/benchmark functions | 349 |
| Go packages | 92 |
| `internal/aphelion` packages | 21 |
| Rust files | 3 |
| Change from baseline | 344 files, 38,324 insertions, 200 deletions |

## Findings

### ADMM-AUDIT-001 — High — Hosted snapshots can crash a joining desktop

`Snapshot.Hash` rejects only non-positive dimensions; it establishes no maximum dimension, maximum tile volume, or checked product (`internal/aphelion/collab/model/hash.go:86-88`). Any authenticated hosted identity can submit a snapshot to session creation, and the service passes it to document startup after that weak hash validation (`internal/aphelion/collab/server/hosted.go:217-243`).

Desktop projection then computes `MaxX * MaxY * MaxZ` directly as a slice or map capacity and loops the full volume (`internal/aphelion/collab/mapadapter/export.go:39-42`, `internal/aphelion/collab/mapadapter/export.go:83-86`). `Editor.AttachCollaborationExecutor` applies the received snapshot (`internal/app/ui/cpwsarea/wsmap/pmap/editor/collaboration.go:96-108`). A small JSON snapshot with `max_x` near `math.MaxInt` and `max_y` equal to `2` can overflow the capacity expression and panic before any large body limit is relevant. Smaller but still excessive dimensions can force unbounded CPU and memory consumption.

Impact: an authenticated session owner can send an invitation that crashes or hangs a participant's desktop. The same unchecked volume affects local export and map application.

Required repair: define protocol-level dimension and total-cell limits, use checked multiplication, enforce the limits in model validation, HTTP creation, snapshot replacement, persistence recovery, and map projection, and publish the same maxima in OpenAPI.

### ADMM-AUDIT-002 — High — Ambiguous durable commits can split store and memory state

The document owner clones the current document, calls `store.Append`, and advances the in-memory document only when `Append` returns `nil` (`internal/aphelion/collab/server/document.go:237-252`). PostgreSQL commits with the request context and may return an error after the server can no longer prove whether the transaction committed (`internal/aphelion/collab/store/postgres/store.go:209-212`). SQLite exposes the same interface-level ambiguity around commit (`internal/aphelion/collab/store/sqlite/store.go:182-185`).

On retry, `submit` returns the stored operation as a duplicate but explicitly retains the old in-memory document (`internal/aphelion/collab/server/document.go:230-235`). The client can receive revision `N` while the document owner still reports revision `N-1`; later operations, snapshots, and broadcasts can diverge.

Impact: loss of authoritative in-process convergence after an interrupted or ambiguous append, despite the durable operation being present.

Required repair: add a fault store that persists and then returns an error, reproduce the split, reconcile append outcomes with a bounded context independent of the canceled caller, and rebuild the in-memory document from the store whenever a duplicate is ahead of memory.

### ADMM-AUDIT-003 — High — The contracted export checkpoint does not exist

OpenAPI advertises `POST /v1/sessions/{session_id}/exports` as a staged export checkpoint (`api/collaboration/openapi.yaml:227-245`). The registered handler always returns `501 Not Implemented` (`internal/aphelion/collab/server/http.go:279`, `internal/aphelion/collab/server/http.go:506-507`). There is no handler test; contract coverage only checks that the route text is present and that the YAML parses (`internal/aphelion/collab/protocol/envelope_test.go:232-250`).

The operation model also implements only `tile_change` and `inverse`, not the approved `export_checkpoint`, `map_resize`, `environment_replace`, or `import_replace` maintenance operations (`internal/aphelion/collab/model/operation.go:5-10`). Map resize is deliberately disabled during collaboration, but export is still represented as implemented in current readiness documents.

Impact: callers following the public contract cannot create a durable checkpoint or begin the staged Meridian acceptance flow. Cross-stack acceptance cannot be complete.

Required repair: implement an owner-authorized, revision/hash-preconditioned durable checkpoint; persist its status and immutable metadata; expose retrieval or a stable completion mechanism; and replace string-presence contract tests with semantic route/handler conformance.

### ADMM-AUDIT-004 — High — Meridian staging and verification do not compose

`Stager.Stage` publishes `stageRoot/<manifest-hash>/map.dmm` (`internal/aphelion/integration/meridian/stage.go:113-147`). `AcceptanceVerifier.Verify` first requires the file to be under `stageRoot`, then requires that same file to equal the configured repository target under the Meridian-Rift checkout (`internal/aphelion/integration/meridian/verify.go:137-148`). Those locations are different in the normal configuration.

The unit fixture hides the mismatch by configuring the repository target itself as `.aphelion-stages/candidate.dmm` (`internal/aphelion/integration/meridian/verify_test.go:157-190`). The shipped PowerShell coordinator does not call `AcceptanceVerifier`; it invokes a real test as a command and later manually copies the staged file into a detached worktree (`scripts/integration/verify-aphelion-stack.ps1:322-341`).

Impact: the advertised Go staging and verification boundary is not an executable end-to-end integration. Tests validate two artificial halves rather than the production composition.

Required repair: make `AcceptanceVerifier` accept a `StagedArtifact`, validate its manifest and stage-root containment, configure MCP inspection against that immutable artifact, and expose a real command entry point. Replace `go test` as production orchestration.

### ADMM-AUDIT-005 — Medium — Current lint is red and releases do not depend on quality/security gates

`golangci-lint run` currently reports four unchecked `NetworkExecutor.Receive` errors:

- `internal/aphelion/collab/client/conformance_test.go:118`
- `internal/aphelion/collab/client/executor_test.go:50`
- `internal/aphelion/collab/client/executor_test.go:84`
- `internal/aphelion/collab/client/executor_test.go:119`

The CI lint job therefore has a current source failure. The release job declares only `needs: build` (`.github/workflows/ci.yml:242-246`), so a tag can create an attested draft release even when lint, collaboration resilience, hosted collaboration, vulnerability, container, or toolset-integration jobs fail.

Several third-party actions also use mutable major tags rather than commit SHAs, including checkout, setup-go, golangci-lint, setup-task, rust-cache, artifact upload/download, and release publication (`.github/workflows/ci.yml:9-278`). This is weaker than the repository's pinning policy for security-sensitive dependencies.

Required repair: make lint green immediately. With explicit approval for the protected workflow, require every release-relevant gate and pin each action to a reviewed commit SHA. Keep the release draft behavior as an additional human control, not the only control.

### ADMM-AUDIT-006 — Medium — Native Windows secret-file validation fails open

`secretFilePermissionsAllowed` returns `true` unconditionally when `goos == "windows"` (`internal/aphelion/collab/server/hosted_config.go:199-202`). The repository documentation requires an equivalent Windows ACL, but the native hosted binary accepts a regular secret file regardless of who can read it. Tests explicitly skip Windows permission semantics in adjacent credential handling.

Impact: a native Windows deployment can start with a broadly readable database DSN or OIDC client secret.

Required repair: either inspect the Windows DACL and reject broad access or explicitly reject native Windows file-backed secrets until that check exists. Add Windows-specific tests against temporary files with controlled ACLs.

### ADMM-AUDIT-007 — Medium — Downstream ownership markers and package boundaries have drifted

Compared with baseline `5241698a`, 11 modified inherited Go files contain no `APHELION EDIT` marker:

- `internal/app/command/command.go`
- `internal/app/command/storage.go`
- `internal/app/ui/cpprefabs/menu.go`
- `internal/app/ui/cpsearch/process.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/psettings/map_size.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/psettings/psettings.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/tilemenu/process.go`
- `internal/app/ui/cpwsarea/wsmap/tools/add.go`
- `internal/app/ui/cpwsarea/wsmap/tools/delete.go`
- `internal/app/ui/cpwsarea/wsmap/tools/fill.go`
- `internal/app/ui/cpwsarea/wsmap/tools/grab.go`

Five new implementation files also place Aphelion behavior directly in inherited package trees without an ownership marker: the two collaboration editor/presence files and three atomic-save files. Package-private access may justify a thin inherited adapter, but the current spans are not visibly owned and some are hundreds of lines.

Impact: the next StrongDMM reconciliation is harder to review and more likely to silently lose collaboration behavior.

Required repair: add exact markers to inherited replacements, move separable behavior into `internal/aphelion`, retain only thin adapters where package-private access is necessary, and record an upstream-drift review before reconciliation.

### ADMM-AUDIT-008 — Low — Documented architecture and verification contracts are not mechanically enforced

- `internal/aphelion/collab/model` imports `github.com/google/uuid` (`internal/aphelion/collab/model/ids.go:6`) despite the documented standard-library-only dependency rule.
- `task verify` passes while `golangci-lint run` fails, so the named local verification aggregate does not represent the full configured quality gate.
- Raw `go vet ./...` still reports four inherited `unsafe.Pointer` diagnostics in `internal/platform/gl.go`.
- OpenAPI/AsyncAPI tests validate YAML roots and string presence, not the schema or handler behavior.
- `scripts/integration/verify-aphelion-stack.ps1:304` invokes monolithic `python -m unittest discover` in aphelion-content-tools instead of its bounded repository runner.
- The locally configured `upstream/main` still points to `5241698a` dated 2026-05-08. No network fetch was performed in this audit, so upstream currentness is unknown rather than confirmed.

Required repair: enforce package dependency rules in tests, define a single documented full local gate, adopt semantic contract linting, use the Content Tools bounded runner, and record a fresh upstream-drift comparison before changing inherited code.

## Feature accounting

| Surface | Audit state | Notes |
| --- | --- | --- |
| Deterministic tile operations and canonical hashing | Implemented, tested | Tile changes, preconditions, idempotency, inverse operations, snapshot/replay tests present. Resource bounds are incomplete. |
| Local executor | Implemented, tested | Same operation engine is used for local compatibility mode. |
| WebSocket collaboration | Implemented, tested | Replay high-water, exact duplicate handling, reconnect, presence separation, roles, limits, and slow-consumer behavior have coverage. |
| Desktop collaboration UX | Implemented, automated coverage partial | Panel/controller/editor adapters exist. Real two-window, keyboard, visual, and narrow-layout gates remain human work. |
| SQLite and PostgreSQL persistence | Implemented, tested locally where available | Restart/replay and store conformance exist. Ambiguous post-commit reconciliation is missing. PostgreSQL was not exercised in this audit. |
| Hosted OIDC and invitations | Implemented, tested with fixtures | External OIDC and reference deployment remain unrun. |
| Hosted deployment | Portable single-replica assets present | Container, backup, restore, fault, telemetry, and public-hosting gates were not rerun in this audit. Multi-replica remains intentionally unsupported. |
| Atomic DMM/TGM save | Implemented, tested | Error-returning staged replacement exists. Dimension overflow must be rejected before projection/export. |
| Export checkpoints | Not implemented | Public route always returns `501`. |
| Exclusive maintenance operations | Partially deferred | Map resize is disabled during collaboration; environment/import/export operation kinds are absent. |
| Meridian-MCP and Meridian-Rift integration | Partial, non-composable | Libraries and tests exist, but no shipped end-to-end command uses `AcceptanceVerifier`. |
| Content Tools integration | Explicitly excluded | Cross-stack script still references Content Tools and uses the wrong test entry point. |
| Updater | Secure fail-closed, operationally disabled | No trusted minisign public key is injected by the current release pipeline. |
| Branding and creative assets | Correctly deferred | Inherited StrongDMM naming/assets remain, consistent with the approved non-goal. |

## Verification performed

| Command | Result |
| --- | --- |
| `go version` | `go1.25.13 windows/amd64` |
| `rustup run 1.82.0-x86_64-pc-windows-gnu rustc --version` | `rustc 1.82.0` |
| `task --version` | `3.53.1` |
| `golangci-lint version` | `2.12.2` |
| `go test ./... -count=1` | Passed; inherited ImGui `memset` warning remained |
| `go test -race ./internal/aphelion/... -count=1` | Passed |
| `task verify` with `RUST_TARGET=1.82.0-x86_64-pc-windows-gnu` | Passed Go, Rust, rustfmt, Clippy, parser release build, and static Windows desktop build |
| `go vet ./...` | Failed with four inherited `unsafe.Pointer` diagnostics |
| `golangci-lint run` | Failed with four unchecked test errors |
| `govulncheck ./...` | Zero called/imported vulnerabilities; seven module-only unreachable findings |
| `go run ./cmd/apheliondmm-doctor` | Passed all pinned tool checks |
| `go run ./cmd/apheliondmm-smoke` | Passed map round trip and two-client headless smoke |
| `actionlint` | Passed |
| `git diff --check` | Passed |

## Explicitly unrun or unavailable gates

- GitHub Actions at `c3469c60`; local results are not hosted CI evidence.
- PostgreSQL conformance/restore with `APHELION_POSTGRES_TEST_DSN` and `APHELION_POSTGRES_BIN`.
- Container-tagged hosted image lifecycle and Trivy image scan.
- Pilot-tagged 25-editor public-contract profile.
- Real Meridian-MCP staging and Meridian-Rift DreamMaker/build acceptance.
- Content Tools repository gates.
- External OIDC, public tunnel/DNS, backup/rollback rehearsal, telemetry alerts, and named-user pilot.
- Real two-window desktop, save/reopen, accessibility, and ordinary single-user regression testing.
- Upstream network refresh; only the local `upstream/main` ref was compared.

## Recommended order

1. Block the snapshot crash path and repair ambiguous commit reconciliation.
2. Restore a green lint/CI baseline.
3. Implement the export checkpoint and make Meridian staging/verification one real entry point.
4. Close the Windows secret ACL gap.
5. Repair ownership markers and dependency-boundary enforcement before any upstream reconciliation.
6. With explicit protected-file approval, harden release dependencies and action pinning.
7. Rerun container, PostgreSQL, cross-stack, hosted, and human gates; then update readiness claims from fresh evidence.
