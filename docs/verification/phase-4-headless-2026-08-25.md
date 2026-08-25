# Phase 4 headless collaboration verification

Date: 2026-08-25

This report records the verified headless and service-facing portion of Phase 4. It is not Phase 4 desktop acceptance.

## Verified

- Connection-state reducer covers disconnected, connecting, synchronizing, caught-up, reconnecting, read-only, conflict, and closed states.
- WebSocket transport uses one reader and one writer, bounds durable submissions, coalesces presence, preserves server close codes, enforces dial deadlines, rejects malformed envelopes, refuses cleartext non-loopback endpoints, refuses credential-bearing URLs, and stops on cancellation.
- Local and network executors pass the same conformance suite for authenticated actor binding, forward execution, actor-scoped inverse, revision advancement, snapshot convergence, and cancellation. Network-specific tests additionally cover submission, authoritative acceptance, rejection rollback, compatible pending rebase, revision ordering, and hash convergence.
- Editor submissions return immediately for network executors, add undo history only after acknowledgement, and apply immutable acknowledged/speculative projections at the start of the map UI frame. Transport termination releases pending callbacks, discards speculative projections, and retains the last acknowledged snapshot for UI rollback.
- The UI scheduling queue accepts concurrent producers without races or lost jobs. Remote executor attachment validates the loaded environment hash before replacing the local snapshot.
- Rejections include stable engine codes and bounded authoritative tile values. The two-client service test verifies `precondition_failed` and the current value for the conflicting coordinate.
- Embedded lifecycle covers one-time launch-token removal from the service object, HTTP session creation, authenticated snapshot fetch, WebSocket join, replay completion, clean leave, service shutdown, token redaction, and connection-loss transition to reconnecting.
- Pure UI models cover accessible text state, role capabilities, deterministic participant ordering, bounded conflicts, sensitive-value redaction, safe conflict actions, and bounded/expired/off-level presence overlays.
- Reconnect policy uses capped exponential backoff with bounded jitter, carries the acknowledged revision, honors cancellation, and stops on authentication or protocol incompatibility.
- The desktop now exposes a neutral Collaboration menu and docked text panel. Local-session creation snapshots only committed editor state, performs service/network work off the UI thread, revalidates the originating editor before UI-thread attachment, and cleans up stale or failed attachments.
- Leaving a desktop session refuses outstanding edits or acknowledgements, restores a local executor from the synchronized snapshot, and then shuts down the client/service without leaving the editor bound to a terminated network executor.
- Controller reservations now reject activation after a concurrent leave; the deterministic race regression and controller race suite pass. The new collaboration layout node increments layout state from 1 to 2, so existing saved layouts reset once to register the docked panel.
- Workspace, environment, and application close/replacement now use a two-stage permit. The controller refuses replacement while operations await acknowledgement, and after an inherited save/discard prompt returns it revalidates the exact session generation and pending-operation state immediately before workspace disposal. Canceling the prompt leaves the session attached.
- Remote cursor presence is converted from cloned session state into capped, expiring, active-level-only overlay commands during the UI frame. The canvas applies its camera transform and inverted Y axis, clips markers to the map view, and draws a text label with one of eight deterministic muted style slots. Connection loss clears observed presence immediately; the overlay path has no map or save mutation access.

## Commands and results

Pinned full gate:

```text
task verify
```

Result: success. This ran all Go tests, Rust 1.82 tests, `cargo fmt --check`, Clippy with `-D warnings`, the release parser build, and the Windows desktop build. The inherited ImGui `imgui_draw.cpp` `memset` warning remains.

Pinned race gate:

```text
go test -race ./internal/aphelion/collab/ui ./internal/aphelion/collab/client ./internal/aphelion/collab/server -count=1
go test -race ./internal/aphelion/collab/client ./internal/aphelion/collab/ui ./internal/app/window ./internal/app/ui/cpwsarea/wsmap/pmap/editor -count=1
go test -race ./internal/aphelion/collab/ui ./internal/app/ui/cpwsarea -count=1
go test -race ./internal/aphelion/collab/ui ./internal/app/ui/cpwsarea/wsmap/pmap -count=1
```

Result: success.

Pinned lint gate:

```text
golangci-lint run ./...
```

Result: `0 issues.`

Fresh produced-executable smoke:

```json
{"status":"ok","session_id_present":true,"distinct_actors":true,"accepted_revisions":[1,2],"client_a_hash":"198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa","client_b_hash":"198873938131e50db17a68ec9fa9006bbf9ab62ad8fa4c5bc430e6adcc7c24aa","leave":"ok","shutdown":"ok"}
```

The produced `apheliondmm-smoke.exe` now runs parser open/save/reparse plus a real embedded service, two distinct authenticated clients, two accepted operations, hash convergence, clean client leave, and clean service shutdown. Fixed smoke-only operation and prefab identities make the final hash reproducible across independent runs. This is still headless evidence; it does not automate Dear ImGui interaction in `StrongDMM.exe`.

## Not yet verified or complete

- Create-local and leave actions are attached to the selected/open map. Join and reconnect actions are not registered because the scoped invitation and credential-resumption UX remains unresolved.
- The collaboration panel shows session identity, role, revision, explicit synchronization state, participants, bounded conflict text, errors, and leave. Copy-invite and reconnect controls are not exposed until their credential contracts are safe.
- Remote presence is rendered by the inherited canvas, but local cursor/selection publication is not connected because protocol v1 does not yet negotiate a presence rate. The desktop visual result still needs manual playtesting.
- Project/environment replacement is wired through inherited asynchronous save/discard dialogs and covered by controller and workspace-level tests. Interactive desktop confirmation remains part of the unperformed manual playtest.
- Reconnect cannot be completed end to end without a credential-resumption decision. The current join credential is deliberately not retained after join, while the wire contract does not issue a separate resumption credential.
- Actor-scoped inverse works through the executor, but inherited desktop undo/redo switching is not yet connected to an active network session.
- No manual keyboard, accessibility, narrow-panel, desktop two-process, or visual playtest has been performed.
