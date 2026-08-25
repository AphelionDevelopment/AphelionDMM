# AphelionDMM agent instructions

AphelionDMM is an Aphelion-owned downstream of StrongDMM. Preserve StrongDMM history and keep the editor usable while introducing deterministic, authoritative multiplayer.

## Required reading

Read the guidance relevant to the task before changing code:

- `docs/agent/README.md` for routing.
- `docs/agent/source-authority.md` for ownership and source precedence.
- `docs/agent/architecture.md` for current and target boundaries.
- `docs/agent/verification.md` for evidence and completion claims.
- `docs/agent/security-and-networking.md` before networking, authentication, updater, path, or process changes.
- `docs/agent/multiplayer-invariants.md` before map mutation, undo, persistence, protocol, or collaboration changes.
- `docs/agent/upstream-drift.md` before importing or reconciling StrongDMM changes.
- `docs/agent/generated-and-external-assets.md` before generated files, branding, fonts, images, sound, or third-party assets.
- `docs/superpowers/specs/2026-08-24-multiplayer-design.md` for the approved multiplayer design.

## Workflow

- Inspect existing implementations with `rg` before designing or editing.
- Preserve unrelated working-tree changes. Do not reset, checkout, merge, commit, or push without explicit user authorization.
- Use PowerShell for local commands on Windows.
- Add tests before implementation for behavioral changes and run the narrow test before the broad gate.
- Test shipped entry points, not only helpers.
- Report untested work plainly. Focused tests are not full completion evidence.

## Ownership and placement

- New Aphelion-owned Go code belongs under `internal/aphelion/`.
- New Aphelion-owned commands belong under `cmd/apheliondmm-*`.
- New public protocol contracts belong under `api/collaboration/`.
- New tests live beside the code they exercise, except end-to-end fixtures under `testdata/`.
- Do not bulk rename module `sdmm`, upstream packages, comments, or StrongDMM identifiers. Branding changes require human approval and a separate migration plan.
- Prefer narrow adapters over copying or replacing inherited StrongDMM subsystems.

When an inherited file must change, mark only the Aphelion-owned span:

```go
// APHELION EDIT ADDITION START - COLLABORATION
// Aphelion-owned code.
// APHELION EDIT ADDITION END
```

For a one-line replacement:

```go
// APHELION EDIT CHANGE - COLLABORATION - ORIGINAL: originalCall()
replacementCall()
```

For an unavoidable removal, retain the original text in a block comment:

```go
/* APHELION EDIT REMOVAL START - COLLABORATION
originalCall()
APHELION EDIT REMOVAL END */
```

Do not convert inherited upstream markers or comments in bulk.

## Protected infrastructure and authorship

Build, bootstrap, release, deployment, signing, and CI entry points are human-authored critical infrastructure. Before changing one, identify the exact file and effect, explain why an Aphelion-owned wrapper cannot provide the extension, and obtain explicit user confirmation.

Do not create or select art, sound, lore, descriptions, item names, or branding. Preserve source URL, author, license, attribution, and human approval for external assets. Agents may build the infrastructure that loads or validates those assets.

## Multiplayer baseline

- The server is authoritative for durable map state.
- Durable edits are explicit, deterministic, idempotent operations; presence is ephemeral and separate.
- Every accepted operation receives a monotonically increasing document revision.
- Undo submits a new actor-scoped inverse operation with preconditions.
- Environment and map hashes are protocol inputs, not advisory metadata.
- Map saves are staged, validated, and atomically replaced. Unknown types must never be silently discarded.
- The local single-user path uses the same operation engine as network collaboration.
- No arbitrary filesystem paths, shell commands, secrets, or executable locations cross the collaboration protocol.

