# Windows Relay Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the protocol-v2 Docker deployment with a self-contained, hardened native Windows service package.

**Architecture:** `apheliondmm-relay.exe` supports both console and SCM execution. In service mode it starts the loopback relay, waits for readiness, and supervises a pinned Cloudflare connector child; one PowerShell entry point packages, installs, operates, updates, and removes the service.

**Tech Stack:** Go 1.25.13, `golang.org/x/sys/windows/svc`, PowerShell 7/Windows PowerShell, Windows SCM/Event Log, Cloudflare Tunnel 2026.7.3.

**Spec:** `docs/superpowers/specs/2026-08-27-windows-relay-service-design.md`

## Global constraints

- No Docker dependency or protocol-v2 Compose artifact remains.
- Do not touch the existing `Cloudflared` service or `bark` tunnel.
- Bind relay and metrics to loopback and create no inbound firewall rule.
- Keep `relay.yaml` non-secret and the tunnel token in a separate ACL-protected file.
- Leave protocol-v1 deployment history unchanged.
- Keep all changes uncommitted and unpushed.

---

### Task 1: Shared relay runtime and process supervisor

**Files:**
- Create: `internal/aphelion/collab/relayruntime/runtime.go`
- Create: `internal/aphelion/collab/servicehost/host.go`
- Create: `internal/aphelion/collab/servicehost/host_test.go`
- Modify: `cmd/apheliondmm-relay/main.go`

- [x] Write service-host tests proving readiness precedes connector startup, cancellation stops both components, and unexpected component exit fails the host.
- [x] Run the narrow tests and confirm the missing implementation failure.
- [x] Move the existing HTTP runtime without changing console behavior and implement the minimal supervisor.
- [x] Run service-host, relay-command, smoke, and race tests.

### Task 2: Native Windows SCM adapter

**Files:**
- Create: `cmd/apheliondmm-relay/service_windows.go`
- Create: `cmd/apheliondmm-relay/service_other.go`
- Create: `cmd/apheliondmm-relay/service_windows_test.go`

- [x] Write Windows tests for service argument validation, stop/shutdown handling, and SCM state transitions.
- [x] Cross-compile to confirm the missing adapter failure.
- [x] Implement `svc.IsWindowsService`, `svc.Run`, LocalService-compatible event logging, and graceful stop/shutdown.
- [x] Run Windows and non-Windows command gates.

### Task 3: Windows installer and package

**Files:**
- Delete: `deploy/relay/Dockerfile`
- Delete: `deploy/relay/compose.yaml`
- Delete: `deploy/relay/compose.cloudflare.yaml`
- Delete: `deploy/relay/compose.loopback.yaml`
- Delete: `deploy/relay/.env.example`
- Delete: `deploy/relay/operations.ps1`
- Create: `deploy/relay/service.ps1`
- Modify: `deploy/relay/relay.yaml.example`
- Modify: `deploy/relay/README.md`
- Modify: `.gitignore`
- Create: `internal/aphelion/collab/windowsservice/package_test.go`
- Delete: `internal/aphelion/collab/container/relay_config_test.go`
- Delete: `internal/aphelion/collab/container/relay_integration_test.go`

- [x] Write tests that execute `Setup`, `Validate`, and `Package` against temporary roots, reject stale package overwrite and malformed tokens, and verify the output contains no Docker/Compose artifacts.
- [x] Run the tests and confirm failure because the installer does not exist.
- [x] Implement the fixed-layout package, pinned Cloudflare download verification, ACLs, service registration/recovery, operations, token-aware rollback, and conservative uninstall.
- [x] Run package tests and validate the downloaded connector's pinned hash and Authenticode signature.
- [ ] Run a real SCM install/start/update/uninstall lifecycle when elevation is available or through the protected Windows CI job.

### Task 4: Protected CI and Task entry points

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `Taskfile.yml`

- [x] Replace protocol-v2 Compose/image steps with Windows build/package/service gates while preserving Linux protocol/race gates and legacy v1 jobs.
- [x] Remove `build-relay-image`; add `package-relay-windows`; keep the default editor build unchanged.
- [x] Validate workflow syntax, Task commands, cross-compilation, and package output.

### Task 5: Documentation and Docker cleanup

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/agent/security-and-networking.md`
- Modify: `docs/agent/verification.md`
- Modify: `docs/hosting/client-owned-relay-agent-handoff.md`
- Modify: `docs/hosting/game-server-deployment-agent-handoff.md`
- Modify: `docs/testing/client-owned-relay-human-test-guide.md`
- Modify: `docs/superpowers/specs/2026-08-26-client-owned-relay-design.md`
- Modify: `docs/superpowers/plans/2026-08-26-client-owned-relay.md`
- Modify: `docs/superpowers/plans/2026-08-26-final-multiplayer-progression-sheet.md`
- Modify: `docs/superpowers/plans/README.md`
- Create: `docs/verification/windows-relay-service-automated-2026-08-27.md`

- [x] Mark the Docker deployment evidence as superseded without rewriting protocol-v1 history.
- [x] Replace operator, server-agent, verification, and human-test commands with the native service workflow.
- [x] Remove ignored protocol-v2 Docker setup remnants after verifying their absolute paths are inside this worktree.
- [x] Run active-deployment dependency, privacy, path, diff, and secret scans.

### Task 6: Full verification and handoff

- [x] Run formatting, focused Go, repository Go, race, Rust/parser, Windows editor, and relay package gates.
- [x] Run dependency, workflow, secret, and diff checks.
- [x] Record hashes, tool versions, tested service lifecycle level, and exact remaining dedicated-host actions.
- [x] Leave the worktree uncommitted and unpushed.
