# Verification

## Evidence levels

Report the exact level reached: static review, focused test, repository test, cross-stack build, shipped-entry-point exercise, public two-network pilot, and cross-repository acceptance. A focused green test is not a desktop or public deployment result. Record exact commands, exit codes, versions, hashes, and known gaps.

## Current toolchain

Use the versions selected by `go.mod`, CI, and the Task files. Record `go version`, `rustc --version`, `task --version`, Docker Engine/Compose versions, and `$LASTEXITCODE` after native Windows commands. The shipped editor remains `dst/StrongDMM.exe` until a separately approved branding change.

## Protocol-v2 narrow gates

```powershell
go test ./internal/aphelion/collab/protocolv2 ./internal/aphelion/collab/identity ./internal/aphelion/collab/store/sqlite -count=1
go test ./internal/aphelion/collab/authority ./internal/aphelion/collab/replica -count=1
go test -race ./internal/aphelion/collab/relay ./internal/aphelion/collab/relayclient -count=1
go test -race ./internal/aphelion/collab/load ./internal/aphelion/smoke -count=1
go test ./internal/aphelion/collab/ui ./internal/app/... -count=1
```

Required automated behaviors include golden wire/invitation compatibility, invalid signatures and replays, one-use/bound admissions, deterministic owner ordering, local durability, transactional snapshot/replay, viewer rejection, conflict/inverse behavior, relay restart from clients, 2/8/32-client opaque routing, privacy assertions, and command-level health/routing smoke.

## Broad and shipped gates

```powershell
go test ./... -count=1
go test -race ./internal/aphelion/... -count=1
task test-rust
task build
Get-FileHash .\dst\StrongDMM.exe -Algorithm SHA256
```

Run the relay command with a real YAML file, verify `/v1/health/live`, `/v1/health/ready`, `/v1/version`, and `/metrics`, route encrypted owner/editor traffic, restart only the relay, and prove clients reconstruct the room without server persistence. Container evidence must use the actual built image and Compose overlays, not only schema parsing.

## Human acceptance

The public acceptance gate uses two computers on separate networks against `https://mapping.a13.info`. Verify owner/editor names, non-overlapping and conflicting edits, inverse/redo, owner-offline pause, manual reconnect after relay restart, desync recovery, save/reopen fidelity, viewer restrictions, endpoint confirmation, keyboard/narrow-layout behavior, and absence of secrets or map plaintext in relay logs.

Do not call protocol v2 complete until the UI exposes every tested action and the public pilot passes. In particular, protocol/authority support alone does not establish desktop owner-transfer acceptance.

## Protected gates

Changes to `.github/workflows/ci.yml`, `Taskfile.yml`, `Taskfile_windows.yml`, deployment/bootstrap/release/signing entry points, or updater publication require exact-file/effect review and explicit confirmation before editing.
