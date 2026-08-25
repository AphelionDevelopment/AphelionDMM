# Security and networking

## Trust boundary

Treat every client message, URL, header, repository identifier, map payload, and reconnect cursor as hostile. The server is authoritative and validates authorization, bounds, hashes, revisions, operation form, and message size before mutation.

The collaboration protocol may carry document identifiers and map operations. It must not carry:

- arbitrary local paths;
- shell commands or arguments;
- executable locations;
- access tokens, private keys, or repository credentials;
- unrestricted URLs for server-side fetching;
- raw SQL or storage implementation details.

## Network defaults

- Bind to loopback unless a human explicitly enables a network deployment.
- Use `wss://` and HTTPS outside loopback.
- Validate WebSocket `Origin` against a configured allowlist before upgrade.
- Authenticate the HTTP request before WebSocket upgrade.
- Authorize each session join and durable operation; connection authentication alone is insufficient.
- Set read, write, idle, handshake, and shutdown deadlines.
- Apply byte limits before JSON decoding and collection-count limits after decoding.
- Reject unsupported protocol versions before joining a session.
- Rate-limit durable operations, joins, reconnects, and presence independently.
- Coalesce lossy presence updates. Never let presence backpressure durable acknowledgements.
- Use structured logs with actor/session identifiers but exclude map content, tokens, and sensitive headers.

## Identity and authorization

Embedded loopback mode uses a short-lived, single-use launch token transferred outside URLs. Hosted mode uses OIDC Authorization Code with PKCE and validates issuer, audience, signature, expiry, and nonce. Roles are viewer, editor, and owner:

- viewer: snapshot and presence access;
- editor: viewer rights plus durable map operations and actor-scoped undo;
- owner: editor rights plus session lifecycle, role administration, and exclusive maintenance operations.

The server derives the actor identity from the authenticated principal. Clients cannot assert another actor ID.

## Filesystem and process safety

- Resolve configured roots once, canonicalize them, and reject targets outside those roots.
- Collaboration messages refer to server-side document IDs, not filesystem paths.
- Stage output in the destination directory, flush, validate, then atomically replace the target.
- Preserve the old file if serialization, validation, flush, or replacement fails.
- Do not invoke shells. External adapters use a fixed executable and fixed subcommand set from trusted local configuration.
- Never expose updater or integration credentials to browser code.

## Dependency and update policy

Pin protocol, database, and security-sensitive dependencies. Review advisories before upgrades. The updater must have request timeouts, response-size limits, TLS verification, signed metadata/artifacts, and a non-destructive rollback path.

Embedded SQLite must bundle a release containing the WAL-reset repair, at least SQLite 3.51.3. Hosted persistence uses PostgreSQL. Database choice does not alter operation semantics.

## Abuse and failure handling

Malformed data returns a bounded error and leaves state unchanged. Repeated violations close the connection. Panics are isolated from the document owner loop, recorded without secrets, and do not acknowledge an operation. Shutdown stops new joins, drains acknowledged durable writes, snapshots eligible documents, and closes connections with a reason.

