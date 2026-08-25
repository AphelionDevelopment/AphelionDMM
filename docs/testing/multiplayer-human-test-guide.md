# Multiplayer Human Test Guide

This guide is for the first hands-on test of AphelionDMM multiplayer. It focuses on the desktop behavior that automated tests cannot judge. Aphelion Content Tools is not part of this test.

## Before testing

- Use two Windows desktop sessions or two people if possible. Two editor windows on one computer are acceptable for the first pass.
- Use disposable, byte-identical copies of a small real `.dme` project and `.dmm` map. Do not use the only copy of production work.
- Start both windows from the same AphelionDMM build. Record the Git revision and executable SHA-256 if available.
- Do not paste invitations into public chat, screenshots, or bug reports. An invitation is short-lived and one-use, but still a credential.
- Keep the test local unless the test coordinator provides an approved hosted URL and identity provider.

For a source build, run from the repository root in PowerShell:

```powershell
go run ./cmd/apheliondmm-doctor
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task build
Get-FileHash .\dst\StrongDMM.exe -Algorithm SHA256
```

Launch `dst\StrongDMM.exe` twice. In each window, use **File > Open...** to open the corresponding disposable project/map copy.

## Basic two-person session

1. In the owner window, open **Collaboration > Show Session Panel** and then **Collaboration > Start Local Session**.
2. Confirm the panel shows an Owner role, a session identifier, a revision, and an understandable status such as **Caught up**.
3. Enter the second tester's display name and click **Copy Editor Invite**.
4. Send the invitation privately to the second tester.
5. In the second window, choose **Collaboration > Join Session**, paste the invitation, and click **Join Session**.
6. Confirm both panels list both participants and eventually show the same revision.
7. Try the same invitation a second time. It should be rejected without disturbing the active session.

Report any token appearing in a URL, log message, saved preference, map, or screenshot as a security issue. Redact it before sharing the report.

## Editing checks

Use visible, reversible edits in the disposable map.

1. Owner makes one edit. Confirm it appears in the editor window and the revision advances for both participants.
2. Editor makes a different, non-overlapping edit. Confirm both windows converge again.
3. Both testers select the same tile and make different edits on a countdown. If a conflict occurs, verify that the panel explains it in text and offers **Refresh**, **Discard**, and **Rebuild**. It must not silently overwrite later work.
4. Test **Edit > Undo** from the actor who made the last accepted edit. Confirm only that actor's safe change is reversed.
5. Let the other actor change the same value, then try to undo the older edit. A stale undo should conflict or be refused rather than erase the later change.
6. Test **Edit > Redo** after a successful collaborative undo.
7. Move the pointer and make a selection on different tiles and z-levels. Presence should appear only where relevant, remain readable, and disappear after the participant leaves.

Watch for flicker, duplicate edits, edits that appear and then vanish without explanation, revision numbers moving backward, long UI freezes, or different final map contents.

## Roles and lifecycle

1. From a fresh session, copy a **Viewer Invite** and join with a third window or repeat the test after leaving.
2. Confirm the viewer can observe but cannot make durable edits or create invitations.
3. Leave from the joined window using **Collaboration > Leave Session**. Confirm the owner remains usable and the participant disappears.
4. Create a new editor invite and rejoin. Confirm a used or expired invite is not reusable.
5. Try closing the active map or environment while an edit is awaiting acknowledgement. The app should prevent unsafe replacement or ask for confirmation; it must not silently discard acknowledged work.
6. Leave the session normally in both windows. Confirm ordinary single-user editing, undo/redo, and save still work afterward.

## Save, close, and reopen

1. Wait until both panels show **Caught up** and the same revision.
2. Save only to disposable paths. Do not have two processes write the same file at the same time.
3. Leave the session, save each disposable copy, close both windows, and reopen the maps.
4. Confirm the intended accepted edits remain and rejected/conflicting edits do not appear.
5. If both participants saved separate byte-identical project copies after convergence, compare the resulting `.dmm` files or their SHA-256 hashes. Differences need investigation even if the maps look similar.

## Usability and accessibility pass

- Resize the Collaboration Session panel as narrow as practical. Important state, errors, participants, and conflict actions should remain findable.
- Check that **Caught up**, **Reconnecting**, **Read only**, and **Conflict** are understandable from text without color.
- Navigate menus, the join dialog, invitation name field, and conflict actions with the keyboard where the inherited ImGui controls permit it. Report focus traps or actions that cannot be reached.
- If using a screen reader, report exactly which collaboration status or controls are announced and which are silent. Screen-reader behavior is not yet certified.
- Check high-DPI scaling and multi-monitor behavior if available.

## Optional disruption checks

Only perform these with disposable data.

- Close the joined window during editing, reopen it, and rejoin with a new invitation. The owner's accepted work must remain intact.
- Close the owner window. Joined clients should report disconnection rather than continue as if they are authoritative.
- For an approved hosted pilot, briefly interrupt the client network and verify **Reconnecting** and recovery. Deployment operators, not desktop testers, should interrupt PostgreSQL or restart the hosted service.

## Stop the test immediately if

- an acknowledged edit is lost;
- two caught-up clients show different map contents or hashes;
- a viewer can edit or create invitations;
- a used invitation works again;
- a credential appears in logs, URLs, saved project data, or a report;
- saving corrupts or truncates the previous map;
- the app crashes repeatedly or cannot leave/recover from a session.

Preserve both disposable map copies and logs before restarting. On Windows, use **Help > Open Logs Directory**, or inspect `%APPDATA%\StrongDMM\logs` if the menu is unavailable.

## What to report

Copy this template for each issue:

```text
Title:
Severity: blocker / major / minor / usability
Build revision:
StrongDMM.exe SHA-256:
Windows version and display scaling:
Local or hosted session:
Role: owner / editor / viewer
Number of participants:
Map/project fixture (non-sensitive description):

Steps:
1.
2.
3.

Expected:
Actual:
Last visible session state and revision:
Did both clients show Caught up? yes/no
Was an edit acknowledged before the failure? yes/no/unknown
Could the issue be reproduced? always/sometimes/once
Approximate timestamp with time zone:

Attachments: redacted screenshots, both disposable maps, relevant log excerpts
Secrets removed: yes/no
```

Never include invitation strings, bearer tokens, cookies, OIDC codes, database URLs, or private map content in a public report. For a divergence or data-loss report, keep the original files unchanged and send hashes before sending files.

## Known boundaries for this round

- Content Tools integration is excluded.
- Local sessions are the first desktop target; there is no public session discovery.
- Hosted operation supports one service replica. Multi-replica operation is intentionally disabled until cross-instance event fanout exists.
- External OIDC-provider certification, reference-deployment load/fault testing, dashboards/alerts, rollback rehearsal, named-user pilot, and updater minisign publication are later controlled rollout gates.
- The inherited product name and executable remain `StrongDMM.exe` until a separate branding decision.
