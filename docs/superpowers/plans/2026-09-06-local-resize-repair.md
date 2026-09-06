# Local map resize repair

**Goal:** Make the existing local resize atomic and keep edit history and stable
identities usable across resize undo/redo. Continue the editor/performance audit.
**Spec:** `../specs/2026-08-24-multiplayer-design.md`, especially the explicit
network-resize deferral. No new wire operation or network resize is introduced.

## Design and boundaries

The settings panel submits dimensions to an error-returning editor method before
changing the map. The editor requires a healthy concrete local executor and no
unfinished gesture/submission. Same-size requests do nothing. The owned planner
validates dimension/cell limits before allocating, copies only retained tile
states, and gives added turf/area instances stable IDs once.

Local resize remains exclusive local maintenance, outside the v1 tile-change
wire vocabulary. Each changed size starts a distinct local document identity,
avoiding revision-zero reuse under the prior document ID. Undo/redo reinstalls
the retained local executor for that size, including its operation history and
IDs. A separate history-generation key selects which command context is active;
the asynchronous attachment generation always increases and still fences queued
callbacks. External attach/detach/close replaces the history key. Resize commands
must check their expected context before installation.

Prepare the target snapshot, environment links and mutable display tiles first.
Only after validation succeeds may the editor switch authority/display, clamp
the visible level, reset the active map's tools and refresh rendering. Failures
retain dimensions, authority, display, command stack and settings input. Resize
undo does not synthesize an inverse tile operation or enable remote maintenance;
full server-side resize remains a separate protocol design/qualification task.

## Work and verification

- [x] Reproduce healthy-authority guard failure, failed initialization publishing
  dimensions, unusable pre-resize undo, and changed IDs after expansion redo.
  Evidence: `.artifacts/resize-audit-2026-09-06/authority-red.txt` and
  `workspace-red.txt`.
- [x] Add `internal/aphelion/editing/resize.go` and adjacent tests for shrink/grow,
  sparse state, unknown content, source isolation, new IDs, bounded dimensions
  and missing default paths. The planner consumes a validated snapshot and the
  loaded environment's default turf/area paths.
- [x] Add owned editor `ResizeMap(x, y, z int) error` and local history checkpoint
  installation. Replace settings' mutate-then-commit seam and retain provenance.
  Test settings delegation, retained input/error and no direct map mutation.
- [x] Preserve old edit undo/redo across multiple resizes, post-resize edits,
  failed undo installation and later attachment changes. Verify hashes, IDs,
  document identities, Save guards, selection reset and clamped Z levels through
  the real workspace. Keep asynchronous callback fences monotonic.
- [x] Verify same-size requests make no executor snapshot, history or view change.
  Source returns before snapshot reading; an allocation regression observes zero
  allocations at 100/1,000/10,000 cells with unchanged authority/history/display.
  Changed-size copy attribution and retained-history resource growth remain in
  the repo-wide performance workplan; no whole-editor speedup is claimed.
- [x] Run narrow packages, affected race suites, explicit hidden resize/selection/
  Save tests, maintained `task verify` and rebuilt parser/loopback smoke. Record
  source/binary hashes and leave human/hosted/DB/integration gates separate.

Completed evidence and remaining limits are in
`../../verification/2026-09-06-local-resize-and-history.md`. Both current and
inactive checkpoint hashes are checked before installation, including retryable
failed undo. Network resize remains deferred under the existing approved design.

Use Go 1.25.13 and Rust `1.82.0-x86_64-pc-windows-gnu`. Preserve unrelated dirty
work and leave everything uncommitted. No protected infrastructure changes.
