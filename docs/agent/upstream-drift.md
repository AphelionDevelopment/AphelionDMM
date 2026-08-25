# Upstream drift

## Purpose

AphelionDMM should remain reviewable as a downstream of StrongDMM. Upstream reconciliation must preserve Aphelion multiplayer invariants and make ownership visible.

## Before reconciliation

1. Record local revision, upstream revision, branches, and working-tree status.
2. Preserve unrelated and uncommitted user work.
3. Review upstream changes to editor mutation, snapshot/undo, DMM/TGM parsing and writing, application lifecycle, parser FFI, dependencies, updater, build, and release files.
4. Identify every touched `APHELION EDIT` span.
5. Re-read the approved design and affected agent guidance.

Do not reset, checkout, merge, rebase, commit, or push without explicit user authorization.

## Reconciliation rules

- Import upstream behavior before reapplying a narrow Aphelion adapter when practical.
- Do not resolve conflicts by deleting markers or copying an old whole file.
- If upstream creates a better extension seam, migrate the Aphelion adapter and remove the inherited-file edit in the same reviewed change.
- Preserve protocol behavior unless a versioned migration is approved.
- Treat parser, serialization, updater, CI, release, and signing conflicts as high risk.
- Re-run golden round-trip fixtures when upstream changes DMM/TGM parsing or writing.
- Re-run two-client convergence and reconnect replay when upstream changes editor mutation or undo.

## Drift record

Each reconciliation report records:

- source and target revisions;
- upstream files reviewed;
- Aphelion-owned spans affected;
- behavior retained, adopted, or rejected with reasons;
- protocol or data migrations;
- focused, repository, build, entry-point, and integration evidence;
- unverified platforms or gates.

Review upstream drift before each planned upstream sync and at least monthly while multiplayer development is active.

