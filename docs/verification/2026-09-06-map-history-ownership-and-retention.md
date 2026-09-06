# Map history ownership and discarded-reference audit

Fresh source inspection and failing regressions reproduced cross-tab history
insertion and retained discarded command payloads. Both are repaired in the
uncommitted working tree based on `052e1acb02b790641d63466de40c91d56028383b`.
This is a bounded history audit, not completion of the repo-wide performance or
editor feature workplan.

## Confirmed findings and repair

1. **Undo entered the wrong map after a tab switch.**
   `Editor.pushAcceptedCommand` used the globally active `CommandStorage.Push`
   when an acknowledgement reached the UI thread. Switching from map A to map B
   while A's edit was pending put A's inverse command into B's history. Direct
   local resize of an inactive editor had the same routing defect. Real hidden
   OpenGL workspace regressions reproduced both failures before repair.
2. **Discarded commands retained their captured state.** Undo/redo truncated
   slices without zeroing popped entries, and pushing a new branch truncated
   redo without clearing its backing array. A regression allocated four separate
   8 MiB payloads, captured them in command closures, undid/redid/undid all four,
   and pushed a new branch. All four remained reachable after explicit GC in the
   original synchronous-command case. This is 32 MiB of logical test payload;
   it is not a measured process-memory delta.
3. **Balance copied redo closures into spare undo capacity.** Its asynchronous
   command check iterated `append(stack.undo, stack.redo...)`. Even when it exited
   immediately, the append could retain redo entries in the unused undo backing
   array. The isolated regression cleared pre-existing pop residue first and
   still reproduced the unwanted references.

`command.Storage.Bind(id) Target` now binds a caller to one stack object without
changing the active stack. `Target.Valid()` checks that exact stack lifetime;
`Target.Push(Command) bool` refuses a disposed target even if another map later
opens under the same ID. Empty/null IDs cannot create targets. The editor binds
its map's existing absolute-path stack identity at construction, before tab
activation, and uses it for accepted edits and local resize. This is local UI
identity; no path was added to collaboration messages.

The binding is a narrow marked adapter in inherited command storage because it
must access private stack lifetime/state. No second command subsystem was copied.
UI-thread ownership and the existing attachment/history generations remain in
force. Disposed history prevents new operation commits and resize; queued
callbacks still have the existing close/attachment fence. A late callback cannot
recreate a removed stack or attach to a reopened map.

Pops now zero their source slot, branch creation clears discarded redo entries,
and disposal/Free clear and release both backing arrays. Balance scans the two
slices separately. Live undo/redo entries continue to own their closures; an
unresolved callback may legitimately retain its in-flight command until it ends.
The patch does not truncate live history or change durable operation retention.

## Evidence

Raw logs are under `.artifacts/history-audit-2026-09-06/`.

| Gate | Result |
| --- | --- |
| Before repair | `ownership-red.txt` fails both real-workspace routing cases; `retention-red.txt` fails payload release and the isolated Balance check. |
| Binding API red test | `target-red.txt` fails compilation because Bind/Target did not yet exist. It is not a runtime reproduction. |
| Command storage | Full package passes. Both synchronous and asynchronous command payloads are collected after branching; live entries survive Balance/undo/redo; dispose/Free release payloads while a stale Target remains live. |
| Editor/settings focused tests | Pass, including disposed-history commit/resize refusal and unchanged authoritative revision, snapshot and Save guard. |
| Hidden workspace | Both ownership regressions pass. The network case applies a real engine acknowledgement, undoes the source edit and checks source/other-map hashes. |
| Maintained repository gate | `task verify` passes lint (zero issues), contracts, all Go tests, Rust test/fmt/clippy and the Windows parser/desktop build. Rust has zero unit tests. |
| Affected race suites | Command storage 1.128 s, editor 1.636 s and settings 1.124 s; all passed. |
| Hidden workspace under race | Explicit history, resize, selection and Save suites passed in 5.598 s with `APHELIONDMM_GL_TEST=1`. |
| Produced parser/loopback smoke | Open/save/reparse, authenticated two-client collaboration, revisions 1/2, equal client hashes, leave and natural shutdown passed. |

The first maintained gate found two S1021 declaration/assignment lint issues in
the modified legacy pop functions. They were corrected and the command package
and complete maintained gate rerun successfully. The inherited ImGui C++
`memset` warning remains. The common editor fixture now supplies its actual test
stack ID in `Dmm.Path`; production code does not fall back to the active tab for
unnamed maps.

Toolchains were read directly: Go 1.25.13 Windows/amd64, Rust 1.82.0 GNU,
Task 3.53.1, golangci-lint 2.12.2 and GCC 15.2.0. SHA-256 provenance:

- Desktop: `292e92c6f69646386eeff211caf6e4c1f5bbc407b2099227e9b25dcb49f9499f`.
- Smoke command: `9c1ee147eda10c438e7976ded05879c875819e3f3a48638919e03650573736d2`.
- Smoke input/output: `7d044f7165ca68f32d823f463c6810876c91aa13934d4880fd96810469e9c9be`.
- Both smoke clients: `198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa`.

`manifest.json` records source/document and binary hashes. The smoke command
reports revision `undefined`; its hash and report provide the explicit artifact
identity. It does not exercise the changed history UI; the hidden workspace
regressions provide that evidence. Documentation and the manifest were completed
after the code gates.

## Remaining work and acceptance limits

- Measure retained local executor checkpoints over repeated real resizes,
  branching and close, including post-GC Go heap, native/private bytes, GPU
  resources and handles. Weak-pointer reachability tests establish closure
  release only. They do not establish RSS recovery, latency or frame-rate gains.
- Reproduce history ordering when a new edit is accepted while undo/redo is
  awaiting acknowledgement. The current transfer checks the top command ID;
  ownership binding alone does not prove ordering under intervening pushes.
- Audit saved-state detection after a saved two-edit history is undone once and
  replaced by a different edit at the same depth. Inspect `appliedCommandId`,
  balance and the workspace's authority-based Save/close guards together before
  claiming either a harmless display issue or a data-loss path.
- Continue partial-capture recovery, placement preview, configurable bindings
  and grid steps, navigation and stamp evaluation in the editor workplan.
- Keep the complete performance matrix open: engine/projection copies, durable
  history/recovery, concurrent pressure and reconnect, parser/FFI/icon lifetime,
  renderer/GPU, search, startup/reload, import/export and long lifecycle trials.

Human desktop acceptance, representative-map latency, PostgreSQL, hosted CI and
Meridian integration were not run in this pass. No commit, push or protected
infrastructure change was made.

Reproduce from the repository root with the pinned tools and check every native
command's exit code:

```powershell
go test ./internal/app/command ./internal/app/ui/cpwsarea/wsmap/pmap/editor ./internal/app/ui/cpwsarea/wsmap/pmap/psettings -count=1 -timeout 90s
go test -race ./internal/app/command ./internal/app/ui/cpwsarea/wsmap/pmap/editor ./internal/app/ui/cpwsarea/wsmap/pmap/psettings -count=1 -timeout 120s
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/app/ui/cpwsarea/wsmap -run '^Test(History|Resize|Selection|SaveAcknowledgementBoundaries)' -count=1 -timeout 120s
Remove-Item Env:APHELIONDMM_GL_TEST
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
New-Item -ItemType Directory -Force .artifacts/history-audit-2026-09-06 | Out-Null
go build -o .artifacts/history-audit-2026-09-06/smoke.exe ./cmd/apheliondmm-smoke
& ./.artifacts/history-audit-2026-09-06/smoke.exe
```
