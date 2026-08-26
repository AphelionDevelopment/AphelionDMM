# Client-owned relay human test guide

**Status:** Ready for the public pilot after `mapping.a13.info` is deployed and externally healthy  
**Recommended setup:** Two computers on separate networks, each with the same test build and project revision

## Before testing

Use a disposable copy of a representative map. Record the StrongDMM executable SHA-256, relay endpoint, project/environment revision, local time zone, and which client is Owner, Editor, or Viewer. Do not share invitation links or unredacted collaboration data/log directories in an issue or chat.

Open the same project and map on both computers. The default relay should be `https://mapping.a13.info`. No Cloudflare Access prompt, account membership, browser sign-in, or special client software should be required.

## Basic session

1. On computer A, choose **Collaboration > Start Online Session**, set the owner name, and keep the default relay.
2. Copy an Editor invite and send it privately to computer B.
3. On computer B, choose **Join Session**, paste the invite, choose a different display name, and join.
4. Confirm both clients report the same session, revision, and usable role.
5. Change the owner's name, then the editor's name. Report if either change is disabled, delayed indefinitely, or inconsistent.
6. Make edits in separate map areas from both clients. Confirm both maps converge.
7. Make overlapping edits. Confirm one result is accepted and the other becomes an explicit conflict rather than silently overwriting.
8. Undo and redo accepted work from each actor. Confirm neither action rewinds the other actor's unrelated changes.
9. Save on the owner, close, reopen, and compare map content/hash. Unknown types and variables must survive.

## Roles and endpoint safety

Create a Viewer invite and confirm the viewer can synchronize but cannot mutate. In Collaboration Settings, test a valid custom HTTPS relay and reject non-loopback HTTP. When joining an invite whose endpoint differs from the configured endpoint, confirm the client asks before connecting.

## Owner and relay interruption

1. Disconnect or close the owner while the editor remains open.
2. Attempt an editor mutation. It must not be accepted; the session should show **Owner offline** or reconnecting and retain acknowledged map state.
3. Restart only the relay service. Do not restore any server data.
4. On the owner, select **Retry Reconnect**.
5. After owner recovery, select **Retry Reconnect** on the editor.
6. Confirm both clients reach the same revision/hash and that pending work was not silently resent or duplicated.
7. Repeat with a participant disconnect during active edits.
8. If a deliberate stale snapshot or replay fault is available in the pilot harness, confirm recovery installs a compatible replay or snapshot transactionally.

## UI checks

Try keyboard-only navigation, a narrow window, high DPI, and readable state without relying only on color. Check focus order, error text, collaboration panel overflow, invitation clearing, reconnect control, leave behavior, and ordinary single-user editing before and after a session.

## Privacy checks

The operator should inspect relay logs and metrics after the session. Report any map text, local path, display name, invitation, capability, group key, private key, or client database content. Expected disclosure is zero.

## Excluded from the first pilot

Online owner transfer has protocol, authority, and relay validation but is not yet fully exposed through the desktop. It is deliberately excluded from the first pilot and is not a blocker for starting that pilot. Do not report it as passed or failed; record any unexpected owner-role transition as a defect.

## What to report

For every issue include:

- exact local timestamp and time zone;
- client role and display name category, not a real personal name;
- executable SHA-256 and relay endpoint;
- session revision and map hash shown by each client;
- exact action sequence;
- expected and actual result;
- whether retry, leave/rejoin, or restart recovered;
- sanitized screenshots and logs from both clients;
- whether the final saved maps match.

Remove invitations, tokens, keys, account names, home paths, and private map content before sharing. A crash, permanent reconnect loop, silent overwrite, duplicated accepted edit, hash mismatch, secret disclosure, or inability to save/reopen is a stop condition.
