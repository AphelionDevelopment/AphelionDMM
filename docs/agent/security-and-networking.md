# Security and networking

## Protocol-v2 trust boundary

Treat every endpoint, header, frame, invitation, operation, profile, and reconnect cursor as hostile. The owner validates application semantics. The relay validates only versioned framing, signatures needed for routing controls, room membership, capability digests, bounds, sequence order, and rate limits.

Application messages use the protocol-v2 binary envelope, Ed25519 signatures, and XChaCha20-Poly1305 encryption with the session group key. The relay must remain payload-opaque and must not receive private keys, group keys, raw invitations, map content, file paths, repository credentials, shell commands, SQL, or executable locations.

## Network defaults

- Clients default to `https://mapping.a13.info` and require HTTPS/WSS outside loopback.
- A user-configured relay persists locally. An invitation that names a different endpoint requires explicit confirmation.
- The public relay is reachable without Cloudflare Access, OIDC, or a user account. Possession of a valid invitation is the admission path.
- The origin service and metrics bind only to loopback. Its dedicated Cloudflare connector child provides outbound-only public transport and Cloudflare terminates public TLS.
- Trust `CF-Connecting-IP` or `X-Forwarded-For` only when the direct peer is inside an explicitly configured proxy CIDR.
- Bound handshake, frame, connection, room, and byte rates before expensive decoding or forwarding.
- Use binary WebSocket frames and disable compression to avoid cross-message compression risks.
- Shutdown and relay restart may drop rooms; clients retain all durable state.

## Local secrets and identity

Installation and session keys live behind `identity.Manager` and the platform secret store. SQLite stores opaque secret references, never secret bytes. Do not log local data paths because they may expose account or machine names. Invitations are secrets: clear UI input after parsing and never include the raw value in errors, logs, reports, screenshots, or test fixtures intended for publication.

## Relay operations

The relay needs one non-secret YAML configuration. A Cloudflare deployment additionally needs only a tunnel-token file installed directly on the host. It needs no database, map volume, OIDC secret, backup job, server-side user store, or container runtime. Run the native Windows service as `LocalService`; grant it read access only to its program, configuration, connector, and token files. Do not create an inbound firewall rule or modify another Cloudflare service.

Public health/version responses reveal service and supported protocol versions only. Metrics expose aggregate active connections and room count. Structured logs may include bounded build/revision and network diagnostics but never client plaintext or credential material.

## Protocol-v1 legacy boundary

The legacy hosted service uses OIDC, PostgreSQL, server-side document authority, and backup/restore procedures. Those requirements apply only to protocol v1. Keep its code and operational material labeled so an operator does not install that stack for a protocol-v2 relay.
