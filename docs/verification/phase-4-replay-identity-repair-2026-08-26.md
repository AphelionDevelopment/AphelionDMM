# Replay and identity repair verification

Date: 2026-08-26

The observed `accepted revision is 9, want 10` failure was reproduced as a replay/live-stream race: durable subscription occurred before replay completed, allowing the same accepted revision to arrive once from storage and again from the queued live stream. The server now captures the joined snapshot revision as a replay high-water mark, replays only through that revision, and skips queued durable messages at or below it.

Defense in depth on the desktop accepts only a byte-equivalent duplicate whose retained operation, revision, and current authoritative hash all match. Altered duplicates, wrong hashes, and skipped revisions suspend the executor, close the transport, and enter the existing reconnect/snapshot-recovery path instead of leaving the editor permanently terminal.

Session-scoped names are validated protocol messages tied to the authenticated actor. Embedded owners choose their initial name; every participant can rename itself; hosted changes are written to PostgreSQL by session and actor before the presence update is published. Deliberate leave ignores only normal or already-closed transport results.

Evidence is in the focused tests under `internal/aphelion/collab/client`, `protocol`, `server`, `store/postgres`, and `ui`. The final full-suite, race, Windows build, hosted container, and live desktop-handoff results and hashes are recorded in `phase-7-desktop-hosted-pilot-2026-08-26.md`.
