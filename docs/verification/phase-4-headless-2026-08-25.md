# Phase 4 headless collaboration verification

Date: 2026-08-25

This report records the verified headless and service-facing portion of Phase 4. It is not Phase 4 desktop acceptance.

## Verified

- Connection-state reducer covers disconnected, connecting, synchronizing, caught-up, reconnecting, read-only, conflict, and closed states.
- WebSocket transport uses one reader and one writer, bounds durable submissions, coalesces presence, preserves server close codes, enforces dial deadlines, rejects malformed envelopes, and refuses cleartext non-loopback endpoints and credential-bearing URLs. The setup context controls dialing and joining; after `Connect` returns, the established connection remains alive until explicit close or transport failure.
- Local and network executors pass the same conformance suite for authenticated actor binding, forward execution, actor-scoped inverse, revision advancement, snapshot convergence, and cancellation. Network-specific tests additionally cover submission, authoritative acceptance, rejection rollback, compatible pending rebase, revision ordering, and hash convergence.
- Editor submissions return immediately for network executors, add undo history only after acknowledgement, and apply immutable acknowledged/speculative projections at the start of the map UI frame. Transport termination releases pending callbacks, discards speculative projections, and retains the last acknowledged snapshot for UI rollback.
- The UI scheduling queue accepts concurrent producers without races or lost jobs. Remote executor attachment validates the loaded environment hash before replacing the local snapshot.
- Rejections include stable engine codes and bounded authoritative tile values. The two-client service test verifies `precondition_failed` and the current value for the conflicting coordinate.
- Rejected operations now retain a detached immutable draft. The collaboration panel exposes refresh, discard, and rebuild actions: refresh reapplies the authoritative projection while keeping the conflict open, discard reapplies it and removes only the rejected draft, and rebuild creates a fresh tile-change operation from current authoritative before-values to the rejected intended after-values. Rebuild never force-overwrites and dismisses the original conflict only after acknowledgement.
- Embedded lifecycle covers one-time launch-token removal from the service object, HTTP session creation, authenticated snapshot fetch, WebSocket join, replay completion, clean leave, service shutdown, token redaction, and connection-loss transition to reconnecting.
- Pure UI models cover accessible text state, role capabilities, deterministic participant ordering, bounded conflicts, sensitive-value redaction, safe conflict actions, and bounded/expired/off-level presence overlays.
- Reconnect policy uses capped exponential backoff with bounded jitter, carries the acknowledged revision, honors cancellation, and stops on authentication or protocol incompatibility.
- Successful joins now redeem the credential's WebSocket capability and return an eight-hour in-memory resumption credential scoped to the same session and actor. Reconnect uses a fresh transport, rolls back unacknowledged speculation, replays from the last acknowledged revision, rotates the resumption credential, and returns the existing executor to caught-up state. Transport loss before replay completion promptly re-suspends the executor and restores reconnecting state instead of hanging an attempt. Redeemed join credentials and prior resumption credentials cannot open another WebSocket; resumption credentials cannot mint invitations. Owner access tokens retain their separate HTTP administrative capability.
- Compacted replay fallback is covered at the server, executor, and controller layers. A stale revision receives `snapshot_required` and a service-restart close without partial replay; the client fetches the HTTP snapshot with the rotated credential, rejects incompatible or rollback baselines, and makes the next attempt from the fetched revision.
- The desktop now exposes a neutral Collaboration menu and docked text panel. Local-session creation snapshots only committed editor state, performs service/network work off the UI thread, revalidates the originating editor before UI-thread attachment, and cleans up stale or failed attachments.
- Leaving a desktop session refuses outstanding edits or acknowledgements, restores a local executor from the synchronized snapshot, and then shuts down the client/service without leaving the editor bound to a terminated network executor.
- Controller reservations now reject activation after a concurrent leave; the deterministic race regression and controller race suite pass. The new collaboration layout node increments layout state from 1 to 2, so existing saved layouts reset once to register the docked panel.
- Workspace, environment, and application close/replacement now use a two-stage permit. The controller refuses replacement while operations await acknowledgement, and after an inherited save/discard prompt returns it revalidates the exact session generation and pending-operation state immediately before workspace disposal. Canceling the prompt leaves the session attached.
- Remote cursor presence is converted from cloned session state into capped, expiring, active-level-only overlay commands during the UI frame. The canvas applies its camera transform and inverted Y axis, clips markers to the map view, and draws a text label with one of eight deterministic muted style slots. Connection loss clears observed presence immediately; the overlay path has no map or save mutation access.
- The authenticated `joined` payload now negotiates `presence_interval_ms` in the bounded 16–5000 ms range. The embedded service advertises 100 ms by default, the canvas publishes cursor and normalized same-level selection bounds no faster than that interval, and a trailing flush preserves the newest update when movement stops inside the interval. The transport continues to coalesce queued presence independently of durable submissions. Selection payloads are capped at 4096 tiles.
- Desktop undo and redo now execute collaboration inverse/forward operations asynchronously. The command remains on its original stack until authoritative acknowledgement, a rejection preserves that history, pending history is temporarily disabled, and discard/balance never emits shared inverse operations. Local commands still complete synchronously through the same transactional storage path.
- A headless desktop integration test forces transport suspension while a map edit awaits acknowledgement. The pending callback is released, the acknowledged projection rolls the speculative map change back, the failed edit never enters undo history, and a fresh post-resume edit can be acknowledged and added to history.
- A real embedded-service integration test gates one durable submission immediately before the WebSocket send, forces transport loss, and verifies that the in-flight callback is released without the operation reaching authoritative state. Automatic credential-rotating reconnect retains the same executor; a fresh edit and its actor inverse are then accepted at revisions 1 and 2 and restore the original tile state.
- A combined headless editor/service integration test attaches the real DMM editor adapter to a session client through an injectable transport factory, gates its first durable WebSocket send, forces disconnect, and verifies authoritative UI-thread rollback with no undo entry. It then waits for automatic reconnect, accepts a fresh editor operation through the real embedded service, records it in desktop command history, executes actor-scoped undo, and restores the original DMM tile state. The expected native Windows error dialog is not displayed during the test; its scheduled UI callback is discarded to keep the headless gate non-interactive.
- Initial join and reconnect now publish `Caught up` only after the usable transport and executor are installed. The combined race gate exposed the former window in which replay completion made editing appear available before transport installation; focused and race verification pass with the ordering repaired.
- The inherited add, delete, replace, fill, move, grab, tile-menu, quick-edit, and variable-edit mutation paths were traced through `BeginTileChange` and `CommitOperation`. Map resize still lacks the required exclusive maintenance operation, so its controls and handler now refuse changes while a network executor is attached instead of mutating the DMM and legacy history outside collaboration.

## Commands and results

Pinned full gate:

```text
task verify
```

Result: success after registering the existing pinned Go 1.24.0 and golangci-lint 2.1.5 directories in the user `PATH`. The repository doctor then passed through ordinary command resolution, and `task verify` ran all Go tests, Rust 1.82.0 tests, `cargo fmt --check`, Clippy with `-D warnings`, the release parser build, and the static Windows desktop build. The focused collaboration race suite, lint gate (`0 issues.`), and produced collaboration smoke also pass. The inherited ImGui `imgui_draw.cpp` `memset` warning remains.

Pinned race gate:

```text
go test -race ./internal/aphelion/collab/ui ./internal/aphelion/collab/client ./internal/aphelion/collab/server -count=1
go test -race ./internal/aphelion/collab/client ./internal/aphelion/collab/ui ./internal/app/window ./internal/app/ui/cpwsarea/wsmap/pmap/editor -count=1
go test -race ./internal/aphelion/collab/ui ./internal/app/ui/cpwsarea -count=1
go test -race ./internal/aphelion/collab/ui ./internal/app/ui/cpwsarea/wsmap/pmap -count=1
go test -race ./internal/aphelion/collab/protocol ./internal/aphelion/collab/server ./internal/aphelion/collab/client ./internal/aphelion/collab/ui ./internal/app/ui/cpwsarea/wsmap/pmap -count=1
go test -race ./internal/app/command ./internal/app/ui/cpwsarea/wsmap/pmap/editor -count=1
go test -race ./internal/aphelion/collab/ui ./internal/app/ui/cpwsarea/wsmap/pmap/psettings ./internal/app/ui/cpwsarea/wsmap/pmap/editor ./internal/app/command -count=1
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

- Create-local and leave actions are attached to the selected/open map. Join is not registered because the scoped invitation-entry UX remains unresolved. Reconnect uses a rotating, dedicated resumption credential and starts automatically after transport loss.
- The collaboration panel shows session identity, role, revision, explicit synchronization state, participants, bounded conflict text, errors, retry reconnect, and leave. Retry reconnect remains disabled while an automatic attempt is active and becomes available after transient attempts are exhausted. Copy-invite is not exposed until its delivery contract is safe.
- Remote cursor and selection presence is rendered and locally published at the negotiated rate. Protocol v1 still lacks viewport and tool-preview fields, and the desktop visual result still needs manual playtesting.
- Map resize is unavailable during active network collaboration until the protocol and engine implement its exclusive maintenance operation.
- Project/environment replacement is wired through inherited asynchronous save/discard dialogs and covered by controller and workspace-level tests. Interactive desktop confirmation remains part of the unperformed manual playtest.
- Automatic replay reconnect is covered against the real embedded service, including credential rotation. Explicit user retry after exhausted transient attempts is covered at the controller and view-model layers, compacted replay uses authenticated snapshot fallback, and one combined headless test now covers an in-flight DMM editor operation, forced disconnect, authoritative rollback, credential-rotating reconnect, a fresh acknowledged edit, command history, and actor undo. Dear ImGui interaction and the expected native error dialog remain outside automation.
- Actor-scoped inverse and forward redo are connected to acknowledged desktop command history, and conflict refresh/discard/rebuild controls are wired. Forced-disconnect reconnect and undo now have combined editor/service coverage; a two-process Dear ImGui desktop playtest remains incomplete.
- No manual keyboard, accessibility, narrow-panel, desktop two-process, or visual playtest has been performed.
