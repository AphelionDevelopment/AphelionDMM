# Multiplayer online-pilot user guide

This is the first human test of the desktop hosted flow. Content Tools is excluded. Use disposable copies of the same small project and map in two StrongDMM windows.

The Aphelion client defaults to `https://mapping.a13.info`. The address field remains editable so a tester can use an operator-approved compatible self-hosted service. The Aphelion hostname is public and must not prompt for Cloudflare Access authorization; identity sign-in is performed by the collaboration service's configured OIDC provider.

## Before starting

- Confirm the operator says `https://mapping.a13.info` is healthy. For an approved self-hosted pilot, record the operator-provided HTTPS origin instead.
- Use `dst\StrongDMM.exe`. The current working-tree build SHA-256 is `E04176F1CFFF1D4D7164D754A19947F6CEDA96887FF93C0BFF00F233B87EB19A`.
- Open byte-identical disposable project/map copies in both windows.
- Never include an invitation, bearer token, OIDC code, database URL, or private map content in a screenshot or report.

## Sign in and connect

1. In the first window choose **Collaboration > Sign In to Hosted Service**. Leave the default `https://mapping.a13.info` unchanged and click **Sign In**. For an approved self-hosted pilot, replace it with the operator-provided HTTPS origin.
2. The browser should show **AphelionDMM pilot sign-in**. Enter a unique stable pilot identifier such as `owner-one` and a visible name such as `Owner One`, then continue. Do not reuse that identifier in the second window.
3. Repeat in the second window with a different identifier and name, such as `editor-two` and `Editor Two`.
4. In the first window choose **Collaboration > Start Hosted Session**. Open the collaboration panel and wait for **Caught up**.
5. Enter the second tester's name and copy an **Editor Invite**. Transfer it privately.
6. In the signed-in second window choose **Collaboration > Join Session**, paste the invitation, and join.
7. Confirm both panels show two distinct participants, the same session, and eventually the same revision.

Expected security behavior: the browser callback displays only completion status; credentials remain in memory; an invite is one-use; a hosted invite using cleartext HTTP is rejected.

## Checks to perform

1. Change the owner's display name in the panel. Confirm both windows update it. Repeat for the editor.
2. Make one non-overlapping edit from each window. Confirm both maps converge and revision numbers never move backward.
3. On a countdown, edit the same tile differently. Confirm the conflict is explained and the available action does not silently erase accepted work.
4. Rapidly alternate ten small edits between windows. Watch for duplicate changes, a client stuck on **Reconnecting**, or `accepted revision is ..., want ...` in logs.
5. Briefly interrupt networking for only the second window, or restart that window. Confirm it reports reconnecting and returns to **Caught up** without losing acknowledged edits.
6. Copy a fresh invite, use it once, and confirm reuse is rejected.
7. Leave normally, sign out, and verify ordinary single-user editing and save still work.

If the operator performs a hosted-service restart, also confirm accepted edits recover and renamed hosted participant names remain attached to their session identities.

## Stop immediately if

- an acknowledged edit disappears;
- two caught-up clients show different map content;
- the same revision is applied twice;
- a viewer can edit or create invitations;
- a used invitation works again;
- a credential appears in a URL, log, preference, project file, screenshot, or report;
- StrongDMM crashes repeatedly or cannot leave/recover.

Preserve both disposable map copies and the logs before restarting. Use **Help > Open Logs Directory** or `%APPDATA%\StrongDMM\logs`.

## Report template

```text
Title:
Severity: blocker / major / minor / usability
StrongDMM.exe SHA-256:
Windows version and display scaling:
Roles and unique pilot identifiers used:
Map fixture description:

Steps:
1.
2.
3.

Expected:
Actual:
Last visible state and revision in each window:
Did both clients show Caught up? yes/no
Was the missing/different edit acknowledged? yes/no/unknown
Could it be reproduced? always/sometimes/once
Approximate timestamp with time zone:

Attachments: redacted screenshots, map hashes, relevant redacted log excerpts
Secrets removed: yes/no
```
