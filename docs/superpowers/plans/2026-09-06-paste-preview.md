# Paste preview implementation workplan

Design: [cancellable paste placement](../specs/2026-09-06-paste-preview-design.md).
Work inline, test first, preserve unrelated changes and leave Git operations
uncommitted. The complete feature includes the actual interaction, not just a
planning helper.

- [x] Reproduce current hidden destination ID replacement and clipping through
  real workspace/clipboard code; establish cancellation/confirmation regressions.
- [x] Add an owned placement constructor/lifecycle with fresh stable IDs, immutable
  clipboard source, sparse destination membership, full bounds validation and
  restoration of passed-over/cancelled backgrounds. Keep ordinary move tests green.
- [x] Replace the narrow editor paste path with an attachment-owned preview.
  Guard Save, resize, replacement and other commits while placement is open.
  Bind completion to the originating editor and retain pending-operation rules.
- [x] Connect Ctrl/Cmd+V, cursor preview, click/Enter confirmation and Escape/tool/
  level/tab cancellation. Add toolbar controls and reference entries. Test actual
  shipped action routing, not a manually recreated helper sequence.
  Implementation and tool/Enter routing pass. The subsequent
  [application routing gate](../../verification/2026-09-06-canvas-resize-and-application-routing.md)
  exercises the actual app dispatcher, both Control keys, Enter, Escape and
  rendered focus transitions between two map workspaces.
- [ ] Verify hidden IDs, sparse holes, malformed/bounded templates, non-visible
  levels, text fields, out-of-map targets, repeated Paste, stale handles,
  network acceptance/rejection, exact undo/redo and real Save after completion.
- [x] Measure bounded preview bookkeeping and unchanged-position work; retain
  artifacts and distinguish these from desktop latency/GPU measurements.
- [x] Add floating template rotation/mirroring through the existing preview
  owner and toolbar/shortcut actions. Verify sparse geometry, stable IDs,
  capture failure, text/modifier suppression, level cancellation, network
  acceptance/rejection and exact local/network undo/redo. See the
  [transform qualification](../../verification/2026-09-06-paste-transforms.md).
- [x] Run focused/race, hidden-workspace, `task verify`, rebuilt desktop and
  parser/loopback gates; publish source/binary evidence and update both workplans.

Evidence and explicit limits: [paste placement audit](../../verification/2026-09-06-paste-placement.md).

Open afterward: expanded attachment/later-acknowledgement interactions, configurable
bindings/grid steps, damaged-display recovery UX preserving unsent intent,
navigation/stamps, representative performance and human acceptance.
