# Phase 7 hosted authentication, compatibility, and configuration evidence

Date: 2026-08-25

## Scope

This record covers the hosted OIDC library and session authorization boundary, the protocol compatibility matrix, and immutable hosted configuration parsing. Local signed-provider container acceptance is recorded separately in `phase-7-container-hosted-2026-08-25.md`; external-provider certification remains outstanding.

## OIDC and hosted authorization

- `github.com/coreos/go-oidc/v3` v3.20.0 and `golang.org/x/oauth2` v0.36.0 are pinned.
- Authorization Code uses S256 PKCE, one-use bounded state, nonce validation, signed ID-token verification, issuer/audience/expiry checks, and optional `at_hash` validation.
- The default OIDC HTTP client has a bounded timeout and does not follow redirects.
- Provider issuer and subject determine the stable internal actor identity; display metadata remains separate.
- Provider refresh tokens are not retained.
- Login authorization and collaboration-session authorization are separate interfaces. Session roles are rechecked on join, on each client message, on owner HTTP operations, and periodically while an idle WebSocket remains connected.
- A revoked or expired hosted authorization closes the WebSocket with a policy-violation status.
- Hosted credentials cannot mint embedded-mode join tokens; hosted membership must use durable one-use invitations bound to OIDC identities.

The signed local issuer suite covers discovery, PKCE parameters, issuer, audience, signature, expiry, nonce, `at_hash`, key rotation, state reuse, disabled identity, role changes, logout, and token expiry. It is deterministic test infrastructure, not a manual login against a deployed non-production identity provider.

## Compatibility

- `/v1/version` publishes the explicit compatibility matrix.
- The authoritative snapshot protocol/schema pair is negotiated before WebSocket join completion.
- `apheliondmm-compat` checks client versions or an explicitly declared rolling pair from a bounded strict JSON matrix.
- Checked-in v1 fixtures cover accepted optional acknowledged revision metadata, rejection of changed protocol semantics, and an expected canonical snapshot hash.
- Actual old/new service process continuity during a rolling PostgreSQL restart remains untested.

## Hosted configuration

- YAML decoding is bounded, rejects unknown fields and multiple documents, and validates HTTPS-only public/OIDC/telemetry origins.
- Bind addresses, trusted proxy CIDRs, collaboration limits, OIDC client ID, and database/OIDC secret sources are validated.
- Secrets must come from an absolute regular file or a named `APHELIONDMM_*` environment variable. Inline values and ambiguous sources are rejected.
- Unix secret files reject group/other permissions. Windows regular-file validation is present; deployment-specific ACL enforcement remains an operational container requirement.

## Commands

```text
go test ./internal/aphelion/collab/auth -count=1
go test -race ./internal/aphelion/collab/auth ./internal/aphelion/collab/server -run 'TestManagerReauthorizesRoleForSpecificCollaborationSession|TestHosted' -count=1
go test ./internal/aphelion/collab/compat ./cmd/apheliondmm-compat ./internal/aphelion/collab/server -count=1
go vet ./internal/aphelion/collab/auth ./internal/aphelion/collab/compat ./internal/aphelion/collab/server ./cmd/apheliondmm-compat
go test ./internal/aphelion/collab/... ./cmd/apheliondmm-compat -count=1
```

All commands completed successfully. The broad collaboration command emitted the inherited `imgui-go` C++ `memset` warning and reported no test failures.

Repository-level follow-up also completed successfully:

```text
go test ./... -count=1
govulncheck ./...
$env:RUST_TARGET = '1.82-x86_64-pc-windows-gnu'; task build
```

`govulncheck` found zero reachable vulnerabilities and reported 19 module-only findings in code paths the repository does not call. The build produced `dst/StrongDMM.exe`; it emitted only the same inherited `imgui-go` warning.
