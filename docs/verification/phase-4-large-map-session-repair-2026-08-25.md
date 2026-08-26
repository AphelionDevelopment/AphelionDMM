# Phase 4 large-map local-session repair

Date: 2026-08-25

## Human report

Starting a local collaboration session from the produced Windows desktop executable returned HTTP 413 while `Blueshift.dmm` from Meridian-Rift was active. The application log contained one error and no warnings, panics, fatal entries, or additional errors:

```text
Unable to start collaboration error="create collaboration session returned HTTP 413"
```

The source DMM is 3,905,246 bytes. Collaboration session creation serializes a per-tile snapshot with stable prefab identities, so its JSON request necessarily exceeds the former generic 2 MiB body ceiling.

## Repair

- Initial embedded and hosted session creation use a distinct `MaxSnapshotBodyBytes` limit with a 256 MiB hard ceiling.
- Invitation, authentication, and other administrative request bodies retain the 2 MiB hard ceiling and their smaller hosted configuration.
- Hosted configuration declares `max_snapshot_body_bytes` explicitly and rejects values above the hard ceiling.
- HTTP 413 errors from desktop session creation now include the encoded snapshot request size for diagnosis without exposing map contents.
- OpenAPI declares HTTP 413 for both embedded and hosted session creation.

## Regression coverage

The server regression submits a valid session snapshot larger than the former 2 MiB limit and requires HTTP 201. A separate security test sets a 64-byte snapshot ceiling and requires rejection before decode. The client regression requires an HTTP 413 error to identify the snapshot request size.

The real two-window `Blueshift.dmm` workflow remains the authoritative human confirmation because the repository does not automate Dear ImGui actions or depend on a sibling repository map fixture.
