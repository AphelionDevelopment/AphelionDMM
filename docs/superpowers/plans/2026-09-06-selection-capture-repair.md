# Selection capture ownership repair

Execute inline with the established test-first workflow. Preserve the working
tree; no subagents, Git integration or protected infrastructure changes.

## Problem and contract

Current rotation/mirror capture adds before-states one at a time. If a later
capture fails, those earlier entries remain even though no display tile changed.
Move initialization and destination capture have the same partial-success issue.
Cancelled moves restore contents but do not release capture ownership directly;
the faulted commit path cannot clear the journal. A validated reattachment then
fails its real-pending-edit guard.

Capture bookkeeping belongs to the action that acquired it. Failed preflight
releases only newly acquired, unmodified entries. Failed destination preflight
keeps the previous preview and its original backgrounds intact. Cancellation
restores and releases the move's own entries, without committing unrelated edits.
A faulted move is cancelled on release. Invalid content must still block Save
until a validated authority replacement; cleanup must not simply clear the fault.
Capture cleanup leaves existing pending operations, unrelated captured edits and
history intact. Deliberate attachment replacement keeps its existing generation
rules for history and callbacks.

The destination interleaving regression also reproduced acquisition of a tile
already owned by an unrelated pending edit. Refuse that preview before mutation;
the move's current background entries are already owned and need no recapture.
Do not latch a global capture fault for this ordinary refusal.

## Steps

- [x] Add red regressions for rotation/mirror/move initialization capture failure,
  later destination failure, cancellation with unrelated pending edits, and
  validated recovery. Exercise real map/operation code and hidden workspace paths.
- [x] In `internal/aphelion/editing/move.go`, release successful captures after
  failed initialization and release only new destinations after failed preview.
  On cancellation restore and release owned backgrounds. Keep successful release
  available to the operation commit path.
- [x] In the owned editor selection adapters, roll back only new transform
  captures after failed preflight. Cancel faulted moves, skip commit for explicit
  cancellation, and keep the error/Save guard until valid replacement.
- [x] Prove failure after a previous preview preserves exact contents and bounds,
  passed-over destinations are recaptured, unrelated edits stay pending, and
  accepted network edits survive failed selection capture and later recovery.
- [x] Run focused editing/editor and hidden workspace regressions, affected race
  suites, `task verify`, and the rebuilt parser/authenticated-loopback smoke.
- [x] Record source/binary evidence and update feature/performance workplans.
  Scope allocation claims to actual measurements; no desktop or database gain
  follows merely from releasing failed capture references.

The broader recovery UX for a damaged display with real unsent intent must retain
or export that intent before any explicit discard action. This repair does not
introduce automatic authority replacement over unacknowledged or uncaptured work.
Placement preview, configurable bindings and the repo-wide performance matrix
remain in the main workplans.

Outcome: [verification and remaining scope](../../verification/2026-09-06-selection-capture-and-cancellation.md).
The bounded repair passes affected race suites, 29 hidden workspace tests,
`task verify` and the rebuilt parser/loopback smoke. Actual repair UX, placement
preview and representative performance/human acceptance remain open.
