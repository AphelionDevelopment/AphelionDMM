# Agent guidance index

This directory turns repository ownership, architecture, safety, and verification decisions into task routing rules.

| If the task concerns | Read |
| --- | --- |
| File ownership, inherited StrongDMM behavior, external authorities | `source-authority.md` |
| Existing subsystems or target multiplayer boundaries | `architecture.md` |
| Tests, builds, runtime checks, or completion claims | `verification.md` |
| WebSockets, HTTP, authentication, updater, paths, processes, or secrets | `security-and-networking.md` |
| Operations, conflicts, presence, undo, persistence, or saves | `multiplayer-invariants.md` |
| StrongDMM updates or conflict reconciliation | `upstream-drift.md` |
| Generated protocol files, branding, images, sound, fonts, or third-party assets | `generated-and-external-assets.md` |

The approved design is `docs/superpowers/specs/2026-08-24-multiplayer-design.md`. The phased execution order is `docs/superpowers/plans/README.md`.

If instructions conflict, apply this precedence:

1. Explicit user direction for the current task.
2. Root `AGENTS.md`.
3. The approved design specification.
4. The active phase plan.
5. Existing local conventions in the narrow code area.

Stop and ask when a required decision would alter protocol compatibility, trust boundaries, creative authorship, or protected infrastructure.

