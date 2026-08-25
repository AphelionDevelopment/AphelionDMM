# Phase 7 market and support-library refresh

Date: 2026-08-25

## Release provenance

GitHub's current artifact-attestation workflow uses short-lived OIDC identity and Sigstore-backed certificates to bind release subjects to build provenance. The release job now uses the commit-pinned `actions/attest` v4 action for produced binaries and archives with only `contents`, `id-token`, and `attestations` permissions.

This is additive provenance, not the updater trust root. The updater remains fail-closed until a human-controlled minisign key signs the strict manifest envelope and every artifact. When a hosted image registry is selected, attest the pushed digest and evaluate keyless Cosign signing at that point; do not add registry credentials or an unused signing layer now.

Primary sources:

- <https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations>
- <https://docs.github.com/en/actions/concepts/security/artifact-attestations>
- <https://github.com/actions/attest>
- <https://github.com/sigstore/cosign-installer>

## Telemetry export

The existing v1.45.0 OpenTelemetry API/SDK instrumentation now uses the matching official OTLP/HTTP trace and metric exporters. OTLP keeps the deployment vendor-neutral, and HTTP aligns with the immutable HTTPS-origin configuration. Empty configuration remains no-exporter; configured export uses verified TLS and bounded startup/shutdown.

Do not add a vendor SDK or deploy a collector until the reference environment is selected. At deployment time, keep collector credentials in the orchestrator secret mechanism and add explicit exporter authentication rather than placing headers in the repository configuration.

Primary sources:

- <https://opentelemetry.io/docs/languages/go/>
- <https://github.com/open-telemetry/opentelemetry-go>

## Multi-replica transport

Retain the single hosted replica and stop-then-start updates. PostgreSQL durability does not provide live accepted-operation fanout between service instances. Adding Redis Streams, NATS JetStream, or another broker before a measured multi-replica requirement would increase operational state without closing a current pilot need. Select and test an event transport only when horizontal availability becomes an approved product requirement; until then, overlapping replicas remain forbidden by the rollout runbook.
