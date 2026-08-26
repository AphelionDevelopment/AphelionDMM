# Client-Owned Collaboration Relay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the database-backed hosted collaboration path with protocol-v2 client-owned sessions routed through a stateless, public WebSocket relay while preserving deterministic operation behavior.

**Architecture:** The desktop owner runs the existing deterministic document authority and commits accepted operations to local SQLite before broadcasting them. Every participant keeps a durable SQLite replica and communicates through encrypted, signed protocol-v2 envelopes; the relay sees only bounded routing metadata and keeps rooms in memory. Protocol v1 remains operational and unchanged until the v2 online pilot passes human acceptance.

**Tech Stack:** Go 1.25.13, `github.com/coder/websocket` v1.8.15, standard-library Ed25519, `golang.org/x/crypto/chacha20poly1305`, `modernc.org/sqlite` v1.57.0 / SQLite 3.53.3, strict YAML, Docker Compose, optional Cloudflare Tunnel.

**Spec:** `docs/superpowers/specs/2026-08-26-client-owned-relay-design.md`

## Global Constraints

- Content Tools work is out of scope.
- Protocol v2 payloads are end-to-end encrypted; the relay must never log or persist plaintext payloads, keys, invitations, map hashes, display names, or filesystem paths.
- Only the connected owner client assigns revisions, resolves conflicts, accepts inverses, creates authoritative snapshots, changes roles, or signs authoritative messages.
- Shared mutation pauses whenever the owner is offline. No election, multi-master merge, or editor promotion occurs automatically.
- Every client persists its accepted state and pending submissions in a local SQLite database using one writer connection, WAL mode, and SQLite 3.51.3 or newer.
- The official client defaults to `https://mapping.a13.info`; HTTPS is mandatory outside loopback and the endpoint remains configurable.
- The relay uses one strict YAML file. Cloudflare credentials remain separate secrets.
- Protocol v1, PostgreSQL, OIDC, and the current `deploy/production` stack remain available until protocol-v2 human acceptance.
- Preserve unrelated working-tree changes. Do not reset, checkout, merge, commit, or push without explicit user authorization; task checkpoints below do not authorize commits.
- Before changing `.github/workflows/ci.yml`, `Taskfile.yml`, `deploy/**`, release files, or operator scripts, identify the exact file and effect and obtain the protected-infrastructure confirmation required by `AGENTS.md`.
- New Aphelion-owned Go code belongs under `internal/aphelion/`; inherited StrongDMM edits remain narrow and carry `APHELION EDIT` markers.

---

## File structure

### New protocol and client-authority units

- `internal/aphelion/collab/protocolv2/wire.go`: fixed, bounded relay-visible header and binary codec.
- `internal/aphelion/collab/protocolv2/crypto.go`: XChaCha20-Poly1305 sealing/opening and Ed25519 signing/verification.
- `internal/aphelion/collab/protocolv2/messages.go`: typed encrypted application messages and stable codes.
- `internal/aphelion/collab/protocolv2/invitation.go`: signed one-use invitation URI codec.
- `internal/aphelion/collab/identity/identity.go`: actor identity creation and validation.
- `internal/aphelion/collab/identity/store.go`: secret-store interface and identity manager.
- `internal/aphelion/collab/identity/store_windows.go`: Windows DPAPI protection.
- `internal/aphelion/collab/identity/store_unix.go`: permission-restricted non-Windows fallback.
- `internal/aphelion/collab/authority/document.go`: extracted deterministic single-writer document owner.
- `internal/aphelion/collab/authority/session.go`: local room owner, manifests, admissions, submission handling, and signed responses.
- `internal/aphelion/collab/authority/sync.go`: replay-or-snapshot synchronization decisions.
- `internal/aphelion/collab/replica/session.go`: durable participant state and accepted-message application.
- `internal/aphelion/collab/replica/recovery.go`: bounded replay/snapshot desynchronization escalation.
- `internal/aphelion/collab/relay/registry.go`: in-memory rooms, admissions, actor bindings, owner presence, and TTL.
- `internal/aphelion/collab/relay/router.go`: role-aware bounded routing and queue policy.
- `internal/aphelion/collab/relay/http.go`: health, version, metrics, and WebSocket endpoint.
- `internal/aphelion/collab/relay/config.go`: strict single-file relay configuration.
- `internal/aphelion/collab/relayclient/client.go`: protocol-v2 WebSocket transport and reconnection.
- `cmd/apheliondmm-relay/main.go`: relay executable.

### Existing units extended without replacing protocol v1

- `internal/aphelion/collab/store/sqlite`: add client-session, pending-submission, manifest, admission, and profile tables/methods.
- `internal/aphelion/collab/client`: expose protocol-neutral accepted/rejected projection methods and add paused/desynchronized states.
- `internal/aphelion/collab/ui`: add relay session lifecycle, v2 invitations, configurable endpoint, and status fields.
- `internal/app`: initialize local identity/store, replace hosted sign-in actions with start-online-session actions, and retain embedded v1 local mode during migration.
- `api/collaboration`: add v2 relay and encrypted-application contracts without overwriting v1 contracts.
- `internal/aphelion/smoke` and `internal/aphelion/collab/load`: add real relay paths.
- `deploy/relay`: add the separate pilot-ready relay stack; do not replace `deploy/production` before human acceptance.

---

### Task 1: Protocol-v2 binary, cryptographic, and invitation contracts

**Files:**
- Create: `internal/aphelion/collab/protocolv2/wire.go`
- Create: `internal/aphelion/collab/protocolv2/wire_test.go`
- Create: `internal/aphelion/collab/protocolv2/crypto.go`
- Create: `internal/aphelion/collab/protocolv2/crypto_test.go`
- Create: `internal/aphelion/collab/protocolv2/messages.go`
- Create: `internal/aphelion/collab/protocolv2/messages_test.go`
- Create: `internal/aphelion/collab/protocolv2/invitation.go`
- Create: `internal/aphelion/collab/protocolv2/invitation_test.go`
- Create: `internal/aphelion/collab/protocolv2/testdata/v2/route_owner.hex`
- Create: `internal/aphelion/collab/protocolv2/testdata/v2/route_actor.hex`
- Create: `internal/aphelion/collab/protocolv2/testdata/v2/operation_accepted.hex`
- Create: `internal/aphelion/collab/protocolv2/testdata/v2/editor_invitation.txt`
- Create: `api/collaboration/relay-v2-asyncapi.yaml`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Produces: `protocolv2.RoomID`, `ActorKey`, `Role`, `Route`, `Header`, `Envelope`, `Invitation`, `Seal`, `Open`, `EncodeInvitation`, `ParseInvitation`.
- Consumes: `model.Operation`, `model.AcceptedOperation`, `model.Snapshot`, and existing SHA-256 map hashes.

- [ ] **Step 1: Write codec tests for exact field order, length prefixes, limits, and golden bytes**

```go
func TestEnvelopeGoldenEncoding(t *testing.T) {
	header := Header{Version: Version, Route: RouteOwner, RoomID: roomID(1), MessageID: messageID(2), Sender: actorKey(3), Sequence: 7}
	envelope := Envelope{Header: header, Nonce: nonce(4), Ciphertext: []byte("opaque"), Signature: signature(5)}
	encoded, err := MarshalEnvelope(envelope)
	require.NoError(t, err)
	require.Equal(t, mustRead(t, "testdata/v2/route_owner.bin"), encoded)
	require.Equal(t, envelope, mustUnmarshal(t, encoded))
}
```

Also assert rejection of unknown versions/routes, short fixed fields, ciphertext above `MaxCiphertextBytes`, trailing bytes, zero room/message/sender IDs, and non-monotonic sequence at the connection validator.

- [ ] **Step 2: Run the codec tests and confirm they fail because protocol v2 does not exist**

Run: `go test ./internal/aphelion/collab/protocolv2 -run 'TestEnvelope' -count=1`

- [ ] **Step 3: Implement the fixed binary envelope**

```go
type Header struct {
	Version   uint16
	Route     Route
	RoomID    RoomID
	MessageID MessageID
	Sender    ActorKey
	Recipient ActorKey
	Sequence  uint64
}

type Envelope struct {
	Header     Header
	Nonce      [chacha20poly1305.NonceSizeX]byte
	Ciphertext []byte
	Signature  [ed25519.SignatureSize]byte
}
```

Use network byte order and explicit length prefixes. `Header.AuthenticatedBytes()` is the single canonical AAD/signature representation used everywhere.

- [ ] **Step 4: Write crypto tests for tampering, wrong keys, wrong sender, replay IDs, and nonce uniqueness**

The happy path must sign `AAD || nonce || ciphertext`, verify the Ed25519 signature before decryption, and decrypt only after all fixed-field validation succeeds. Mutating every authenticated header field, ciphertext, nonce, or signature must fail.

- [ ] **Step 5: Implement `Seal` and `Open` with XChaCha20-Poly1305 and Ed25519**

```go
func Seal(random io.Reader, key [32]byte, signer ed25519.PrivateKey, header Header, plaintext []byte) (Envelope, error)
func Open(key [32]byte, expectedSigner ed25519.PublicKey, envelope Envelope) ([]byte, error)
```

Promote `golang.org/x/crypto` to a direct dependency. Never reuse a nonce; tests inject a deterministic reader only for fixtures.

- [ ] **Step 6: Define typed application payloads and strict JSON decoding inside ciphertext**

Define `SyncHello`, `SyncReplay`, `SyncSnapshot`, `SyncComplete`, `OperationSubmit`, `OperationAccepted`, `RevisionAcknowledged`, `OperationRejected`, `InverseRequest`, `ProfileUpdate`, `PresenceUpdate`, `RoleManifest`, `OwnershipOffer`, `OwnershipAccept`, `OwnershipTransferred`, and `SessionNotice`. Every decoder uses `DisallowUnknownFields`, a byte limit, and a trailing-value check.

- [ ] **Step 7: Write and implement signed one-use invitation URI tests**

```go
type Invitation struct {
	Version           uint16
	RelayURL          string
	RoomID            RoomID
	Role              Role
	Admission         [32]byte
	GroupKey          [32]byte
	OwnerPublicKey    ActorKey
	ManifestSHA256    [32]byte
	IssuedAt          time.Time
	ExpiresAt         time.Time
	OwnerSignature    [64]byte
}
```

`EncodeInvitation` returns `apheliondmm://join?invite=<base64url>`. `ParseInvitation` rejects HTTP outside loopback, userinfo, fragments, query-bearing relay URLs, expiry beyond the configured maximum, owner-signature failure, and trailing data.

- [ ] **Step 8: Publish the v2 contracts and run the package gate**

Document relay-visible `room_create`, `admission_replace`, `owner_transfer`, `connect`, routing, heartbeat, and disconnect messages separately from encrypted payload schemas. Run: `go test ./internal/aphelion/collab/protocolv2 -count=1` and `git diff --check`.

- [ ] **Step 9: Review checkpoint**

Confirm golden fixtures contain no real key, invitation, name, or local path. Leave changes uncommitted unless explicit commit authorization is provided.

---

### Task 2: Client identity and protected secret storage

**Files:**
- Create: `internal/aphelion/collab/identity/identity.go`
- Create: `internal/aphelion/collab/identity/identity_test.go`
- Create: `internal/aphelion/collab/identity/store.go`
- Create: `internal/aphelion/collab/identity/store_test.go`
- Create: `internal/aphelion/collab/identity/store_windows.go`
- Create: `internal/aphelion/collab/identity/store_windows_test.go`
- Create: `internal/aphelion/collab/identity/store_unix.go`
- Create: `internal/aphelion/collab/identity/store_unix_test.go`

**Interfaces:**
- Consumes: `protocolv2.ActorKey`.
- Produces: `identity.Identity`, `SecretStore`, `Manager.LoadOrCreate`, `Manager.CreateSessionKey`, and opaque secret references for SQLite.

- [ ] **Step 1: Write identity-manager tests**

```go
type SecretStore interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
}

type Identity struct {
	PublicKey  protocolv2.ActorKey
	PrivateKey ed25519.PrivateKey
}
```

Test stable reload, generate-once behavior, corrupt-secret failure without replacement, cancellation, private-key zeroing of temporary buffers, and separate identity/session/group-key namespaces.

- [ ] **Step 2: Run the identity tests and confirm missing implementation failures**

Run: `go test ./internal/aphelion/collab/identity -count=1`

- [ ] **Step 3: Implement the manager and permission-neutral secret-store contract**

Use random 128-bit opaque record names. Never put a private key, group key, or owner capability in logs or returned errors.

- [ ] **Step 4: Implement Windows DPAPI storage**

Use `CryptProtectData`/`CryptUnprotectData` through `golang.org/x/sys/windows`, with current-user scope and no UI. Store only DPAPI ciphertext under the supplied application-data secret directory. Tests prove plaintext is absent from the file and another record name cannot decrypt it.

- [ ] **Step 5: Implement the non-Windows restricted-file fallback**

Create the secrets directory as `0700` and files as `0600`, reject symlinks/non-regular files, write through a sibling temporary file, `fsync`, and atomically rename. Refuse to load a secret with group/other permission bits.

- [ ] **Step 6: Run platform-neutral and Windows gates**

Run: `go test ./internal/aphelion/collab/identity -count=1` and `go test -race ./internal/aphelion/collab/identity -count=1`.

- [ ] **Step 7: Review checkpoint**

Inspect test output and repository content for private-key bytes or profile paths. Leave uncommitted.

---

### Task 3: Durable client-session SQLite schema and API

**Files:**
- Create: `internal/aphelion/collab/store/client.go`
- Create: `internal/aphelion/collab/store/sqlite/schema/003_client_sessions.sql`
- Create: `internal/aphelion/collab/store/sqlite/client.go`
- Create: `internal/aphelion/collab/store/sqlite/client_test.go`
- Modify: `internal/aphelion/collab/store/sqlite/migrations.go`
- Modify: `internal/aphelion/collab/store/sqlite/store.go`
- Modify: `internal/aphelion/collab/store/sqlite/store_test.go`

**Interfaces:**
- Consumes: existing `store.SessionStore`, `model` snapshots/operations, `protocolv2` role/key types, and opaque identity secret references.
- Produces: `store.ClientSessionStore`, `LocalSession`, `PendingSubmission`, `Admission`, and transactional replica replacement.

- [ ] **Step 1: Define the client-store interface and records**

```go
type ClientSessionStore interface {
	SessionStore
	CreateClientSession(context.Context, LocalSession) error
	LoadClientSession(context.Context, string) (LocalSession, error)
	SavePending(context.Context, PendingSubmission) error
	ResolvePending(context.Context, string, model.OperationID, PendingDisposition) error
	ReplaceReplica(context.Context, LocalSession, model.Snapshot, []model.AcceptedOperation) error
	SaveManifest(context.Context, string, []byte, []Admission) error
	ListAdmissions(context.Context, string) ([]Admission, error)
}
```

`LocalSession` stores relay URL, role, document ID, owner public key, manifest bytes/hash, acknowledged revision/hash, display name, and opaque secret references. It never stores raw private/group/capability keys.

- [ ] **Step 2: Write migration and transaction-failure tests**

Cover schema-2 upgrade, future-schema rejection, session isolation, pending survival after reopen, accepted-before-ack ordering, atomic snapshot replacement, admission consumption/binding, corrupt manifest failure, and rollback after an injected SQL failure.

- [ ] **Step 3: Run the new narrow tests and confirm failure**

Run: `go test ./internal/aphelion/collab/store/sqlite -run 'Test(Client|OpenMigratesClient|ReplaceReplica)' -count=1`

- [ ] **Step 4: Implement schema version 3**

Create `client_sessions`, `pending_submissions`, `actor_profiles`, and `admissions` tables, all keyed/scoped by `session_id`. Keep existing `documents`, `operations`, and `revision_hashes` as the durable map journal.

- [ ] **Step 5: Implement transactional methods**

`ReplaceReplica` verifies the full snapshot/replay/hash chain before beginning its write transaction, then replaces document/session acknowledgement state atomically. `ResolvePending` changes exactly one pending row and the matching conflict state in the same transaction.

- [ ] **Step 6: Verify SQLite safety requirements**

Retain `SetMaxOpenConns(1)`, WAL, `synchronous=FULL`, foreign keys, busy timeout, and runtime version floor 3.51.3. Add a test that `SELECT sqlite_version()` is at least the floor and record the actual version in diagnostics without logging the database path.

- [ ] **Step 7: Run store conformance, race, and corruption tests**

Run: `go test ./internal/aphelion/collab/store/sqlite -count=1` and `go test -race ./internal/aphelion/collab/store/sqlite -count=1`.

- [ ] **Step 8: Review checkpoint**

Open a test database and confirm secrets are references, not raw values. Leave uncommitted.

---

### Task 4: Extract reusable local document authority and add owner sessions

**Files:**
- Create: `internal/aphelion/collab/authority/document.go`
- Create: `internal/aphelion/collab/authority/document_test.go`
- Create: `internal/aphelion/collab/authority/session.go`
- Create: `internal/aphelion/collab/authority/session_test.go`
- Create: `internal/aphelion/collab/authority/manifest.go`
- Create: `internal/aphelion/collab/authority/manifest_test.go`
- Create: `internal/aphelion/collab/authority/sync.go`
- Create: `internal/aphelion/collab/authority/sync_test.go`
- Modify: `internal/aphelion/collab/server/document.go`
- Modify: `internal/aphelion/collab/server/document_test.go`

**Interfaces:**
- Consumes: `engine.Document`, `store.SessionStore`, `store.ClientSessionStore`, identity signer, and protocol-v2 application messages.
- Produces: `authority.Document`, `OwnerSession.Handle`, `OwnerSession.Sync`, `OwnerSession.ReplaceAdmissions`, `OwnerSession.TransferOwnership`, and compatibility wrappers for v1 server callers.

- [ ] **Step 1: Write extraction conformance tests**

Run the existing submit, duplicate, inverse, snapshot, persistence-failure, and concurrent-ordering cases against both `authority.Document` and the v1 `server.DocumentOwner` wrapper.

- [ ] **Step 2: Move the single-writer loop without changing behavior**

```go
type Document interface {
	Submit(context.Context, model.Operation) (model.AcceptedOperation, bool, error)
	Snapshot(context.Context) (model.Snapshot, error)
	BuildInverse(context.Context, model.ActorID, model.OperationID, model.OperationID) (model.Operation, error)
	Close(context.Context) error
}
```

Keep v1 exported constructors as wrappers/type aliases so current server tests remain source-compatible.

- [ ] **Step 3: Write owner-session tests**

Prove role validation, actor-signature validation, commit-before-accept broadcast, owner-name update, safe inverse behavior, idempotent duplicate submission, admission replacement, removal/key-rotation notice, dual-signed online ownership transfer, and no response when the local append fails.

- [ ] **Step 4: Implement the owner manifest and admission lifecycle**

The owner-signed manifest contains room ID, owner key, monotonically increasing manifest generation, actor keys/roles/display profiles, and group-key generation. `ReplaceAdmissions` emits only hashes, roles, expiry, and restored actor bindings to the relay.

- [ ] **Step 5: Implement owner operation handling**

`Handle(OperationSubmit)` verifies the editor signature and manifest role, calls the extracted document authority, looks up the committed revision hash, creates `OperationAccepted`, signs it, and only then hands it to the transport. Convert engine rejections into bounded owner-signed `OperationRejected` values. Validate participant-signed `RevisionAcknowledged` messages and retain peer progress only in owner-client memory.

- [ ] **Step 6: Implement online owner transfer**

The current owner creates an `OwnershipOffer` naming the target actor and a fresh target-generated owner signing public key. The target returns `OwnershipAccept` signed by both its actor key and new owner key. The old owner commits a new manifest generation and emits `OwnershipTransferred` signed by both old and new owner keys. The relay accepts `owner_transfer` only while both connections are live and only after verifying both signatures; it then pauses routing during the atomic owner-key/connection replacement.

- [ ] **Step 7: Implement replay-or-snapshot synchronization**

```go
func (session *OwnerSession) Sync(ctx context.Context, hello protocolv2.SyncHello) (protocolv2.SyncReply, error)
```

Return replay only when the local store contains the hello revision/hash and a continuous later log. Otherwise return the current snapshot. Reject environment mismatch, unauthorized actor, impossible future revision, and oversized pending-ID lists.

- [ ] **Step 8: Run authority and legacy server tests**

Run: `go test -race ./internal/aphelion/collab/authority ./internal/aphelion/collab/server -count=1`.

- [ ] **Step 9: Review checkpoint**

Confirm there is one revision-assignment loop and that every acceptance follows a successful local append. Leave uncommitted.

---

### Task 5: Durable participant replica and bounded desynchronization recovery

**Files:**
- Create: `internal/aphelion/collab/replica/session.go`
- Create: `internal/aphelion/collab/replica/session_test.go`
- Create: `internal/aphelion/collab/replica/recovery.go`
- Create: `internal/aphelion/collab/replica/recovery_test.go`
- Modify: `internal/aphelion/collab/client/executor.go`
- Modify: `internal/aphelion/collab/client/executor_test.go`
- Modify: `internal/aphelion/collab/client/state.go`
- Modify: `internal/aphelion/collab/client/state_test.go`

**Interfaces:**
- Consumes: `client.Projection`, `client.NetworkExecutor`, `store.ClientSessionStore`, owner public key, and decrypted protocol-v2 messages.
- Produces: `replica.Session`, `ApplyAccepted`, `ApplyRejected`, `InstallReplay`, `InstallSnapshot`, `PendingReview`, and explicit paused/desynchronized states.

- [ ] **Step 1: Refactor projection application behind protocol-neutral methods**

Add `NetworkExecutor.Accept(ctx, accepted, mapHash)` and `Reject(ctx, rejected)`; keep v1 `Receive` as a decoder that calls those methods. Existing v1 executor tests must stay green.

- [ ] **Step 2: Write replica durability tests**

Prove that pending submissions are stored before send, accepted operations are stored before visible acknowledgement, restart reloads pending/accepted state, rejected drafts survive restart, duplicate acceptances are idempotent, and wrong owner signatures never reach SQLite or projection state.

- [ ] **Step 3: Implement `replica.Session`**

```go
type Session struct {
	store    store.ClientSessionStore
	executor *client.NetworkExecutor
	ownerKey ed25519.PublicKey
	recovery RecoveryState
}
```

`Submit` stores pending intent, then sends. `ApplyAccepted` verifies owner signature, transactionally appends/acknowledges/resolves pending, then updates the projection and emits `RevisionAcknowledged`. A storage failure blocks the replica and emits no acknowledgement.

- [ ] **Step 4: Add paused and desynchronized states**

Add `StatePausedOwnerOffline` and `StateDesynchronized`, with transitions from synchronizing/caught-up/read-only/conflict. Paused state allows local inspection/export but rejects mutation. Only verified sync completion leaves desynchronized state.

- [ ] **Step 5: Implement one-replay/one-snapshot escalation**

```go
type RecoveryState struct { ReplayAttempted, SnapshotAttempted bool }
func (state RecoveryState) Next(cause IntegrityFailure) (RecoveryRequest, RecoveryState, error)
```

First integrity failure requests replay; failure after replay requests snapshot; failure after snapshot returns a stable terminal diagnostic. Never loop automatically.

- [ ] **Step 6: Implement pending-review classification after sync**

Classify each durable pending operation as `ready`, `conflicting`, or `obsolete` against the new acknowledged snapshot. Do not automatically resend any class.

- [ ] **Step 7: Run replica, client, and race tests**

Run: `go test -race ./internal/aphelion/collab/replica ./internal/aphelion/collab/client -count=1`.

- [ ] **Step 8: Review checkpoint**

Confirm editor state cannot advance ahead of durable local state. Leave uncommitted.

---

### Task 6: Stateless in-memory relay core

**Files:**
- Create: `internal/aphelion/collab/relay/registry.go`
- Create: `internal/aphelion/collab/relay/registry_test.go`
- Create: `internal/aphelion/collab/relay/router.go`
- Create: `internal/aphelion/collab/relay/router_test.go`
- Create: `internal/aphelion/collab/relay/limits.go`
- Create: `internal/aphelion/collab/relay/limits_test.go`

**Interfaces:**
- Consumes: relay-visible protocol-v2 headers/control messages only.
- Produces: `Registry.CreateRoom`, `ReplaceAdmissions`, `Connect`, `Disconnect`, `Route`, `ExpireIdle`, and stable relay errors.

- [ ] **Step 1: Write registry behavior tests**

Cover first-owner creation, owner proof-of-possession, one active owner, UUID room collision, signed admission replacement, one-use actor binding, restored bindings after simulated restart, expiry, revoked admission, dual-signed online owner transfer, owner disconnect pause, owner reconnect resume, and idle room expiry.

- [ ] **Step 2: Implement the registry with no persistence dependency**

```go
type Registry struct {
	mutex sync.Mutex
	rooms map[protocolv2.RoomID]*room
	now   func() time.Time
}
```

The package may import crypto/protocol packages but must not import `model`, `engine`, `store`, SQL, filesystem, OIDC, or map adapters. Add an import-boundary test using `go list -deps`.

- [ ] **Step 3: Write router queue and role tests**

Prove editors route only to owner, viewers cannot route durable submissions, owner can route to one actor or the room, editor durable routing fails with `owner_offline`, presence coalesces at depth one, durable queues never silently drop, and slow consumers close with `slow_consumer`.

- [ ] **Step 4: Implement bounded routing**

Use one bounded durable queue and one depth-one presence slot per connection. Account ciphertext bytes before enqueue. Enforce room/global connection, per-room bandwidth, message burst, and ciphertext size limits.

- [ ] **Step 5: Add opacity and privacy tests**

Send plaintext markers inside ciphertext and assert logs/metrics/registry snapshots never contain them. Assert the registry is empty after process-equivalent reconstruction and after TTL expiry.

- [ ] **Step 6: Run relay core race tests**

Run: `go test -race ./internal/aphelion/collab/relay -run 'Test(Registry|Router|RelayDoesNotRetain)' -count=1`.

- [ ] **Step 7: Review checkpoint**

Use `go list -deps ./internal/aphelion/collab/relay` to confirm no database/map authority dependency. Leave uncommitted.

---

### Task 7: Relay configuration, HTTP/WebSocket service, and executable

**Files:**
- Create: `internal/aphelion/collab/relay/config.go`
- Create: `internal/aphelion/collab/relay/config_test.go`
- Create: `internal/aphelion/collab/relay/http.go`
- Create: `internal/aphelion/collab/relay/http_test.go`
- Create: `internal/aphelion/collab/relay/websocket.go`
- Create: `internal/aphelion/collab/relay/websocket_test.go`
- Create: `cmd/apheliondmm-relay/main.go`
- Create: `cmd/apheliondmm-relay/main_test.go`
- Modify: `cmd/apheliondmm-healthcheck/main.go`
- Modify: `cmd/apheliondmm-healthcheck/main_test.go`
- Create: `docs/hosting/relay-config.schema.json`

**Interfaces:**
- Consumes: `relay.Registry`, `relay.Router`, `github.com/coder/websocket`, trusted-proxy handling patterns, and the approved YAML schema.
- Produces: `relay.LoadConfig`, `relay.NewService`, `/v2/relay`, `/v1/health/live`, `/v1/health/ready`, `/v1/version`, `/metrics`, and `apheliondmm-relay -config`.

- [ ] **Step 1: Write strict configuration tests**

Test the exact approved keys/defaults, unknown-key failure, non-loopback bind warning/error policy, invalid public origin, invalid CIDRs, unsafe limit combinations, zero/negative durations, and configuration redaction.

- [ ] **Step 2: Implement strict YAML loading and JSON Schema parity**

Decode one YAML document with `KnownFields(true)`, reject trailing documents, apply bounded defaults, and make the schema examples validate in tests.

- [ ] **Step 3: Write HTTP and WebSocket tests**

Test origin allowlist, trusted proxy parsing, TLS-forwarded requests, binary-only frames, control-message authentication, heartbeat timeout, close codes, body/header limits, graceful shutdown, liveness/readiness, and version reporting `protocol_versions: [2]`.

- [ ] **Step 4: Implement the service**

Use `websocket.Accept` with compression disabled. Maintain a ping/heartbeat deadline shorter than Cloudflare idle failure windows. Never use request context after hijack; create an explicit connection context.

- [ ] **Step 5: Implement the command entry point**

Require `-config`, reject positional arguments, load configuration before listening, handle SIGINT/SIGTERM, print only bind/build/revision metadata, and shut down within 15 seconds.

- [ ] **Step 6: Make the existing healthcheck command accept v2 relay readiness**

Keep its URL override and bounded timeout. Test both current v1 and new v2 readiness bodies so legacy deployment remains operable.

- [ ] **Step 7: Run command/service gates**

Run: `go test -race ./internal/aphelion/collab/relay ./cmd/apheliondmm-relay ./cmd/apheliondmm-healthcheck -count=1`.

- [ ] **Step 8: Review checkpoint**

Inspect logs for tokens, keys, invitation payloads, display names, and remote payloads. Leave uncommitted.

---

### Task 8: Protocol-v2 relay client and full owner/replica transport loop

**Files:**
- Create: `internal/aphelion/collab/relayclient/client.go`
- Create: `internal/aphelion/collab/relayclient/client_test.go`
- Create: `internal/aphelion/collab/relayclient/owner.go`
- Create: `internal/aphelion/collab/relayclient/participant.go`
- Create: `internal/aphelion/collab/relayclient/e2e_test.go`
- Create: `internal/aphelion/collab/relayclient/restart_test.go`
- Modify: `internal/aphelion/collab/client/reconnect.go`
- Modify: `internal/aphelion/collab/client/reconnect_test.go`

**Interfaces:**
- Consumes: `authority.OwnerSession`, `replica.Session`, protocol-v2 codec, identity keys, and relay WebSocket endpoint.
- Produces: `relayclient.Owner`, `Participant`, `ConnectOwner`, `ConnectParticipant`, and automatic owner room recreation.

- [ ] **Step 1: Write transport tests against `httptest.Server`**

Test HTTPS/loopback policy, origin, binary frames, connect/admission exchange, sequence monotonicity, duplicate message rejection, heartbeat, context cancellation, close reason mapping, and exponential backoff with jitter.

- [ ] **Step 2: Implement the shared WebSocket client**

Expose decrypted application messages only after header validation, signature verification, replay-ID filtering, and AEAD open. Bound the replay-ID cache per sender and session.

- [ ] **Step 3: Implement owner routing adapter**

On connect/reconnect: create the room, publish the complete current admission set, announce resume, and route participant messages through `OwnerSession.Handle`. Owner disconnection must not mutate local authority state. During an approved transfer, keep both connections live until the relay acknowledges the dual-signed `owner_transfer` and the new owner has opened its local authority.

- [ ] **Step 4: Implement participant routing adapter**

Connect with the invitation capability, send `SyncHello`, install replay/snapshot transactionally, send `SyncComplete`, and enter caught-up/read-only. Map `owner_offline` to paused state without destroying the local executor.

- [ ] **Step 5: Write real two-client convergence and relay-restart tests**

Run owner plus editor through a real relay WebSocket. Submit non-overlapping operations, a conflict, inverse, and owner/editor name changes. Restart only the relay, verify owner recreation, editor reconnect, identical revision/hash, and zero server map persistence.

- [ ] **Step 6: Write crash-window tests**

Interrupt after owner SQLite append but before broadcast, after editor SQLite append but before projection update, and during snapshot installation. Reopen stores and prove deterministic recovery at the last committed transaction.

- [ ] **Step 7: Run transport race and repeat gates**

Run: `go test -race ./internal/aphelion/collab/relayclient -count=10`.

- [ ] **Step 8: Review checkpoint**

Confirm relay restart needs no server restore and pending operations are never automatically resent. Leave uncommitted.

---

### Task 9: Desktop lifecycle, configurable relay, invitations, and collaboration UX

**Files:**
- Create: `internal/app/config_collaboration.go`
- Create: `internal/app/config_collaboration_test.go`
- Create: `internal/aphelion/collab/ui/relay_client.go`
- Create: `internal/aphelion/collab/ui/relay_client_test.go`
- Modify: `internal/aphelion/collab/ui/defaults.go`
- Modify: `internal/aphelion/collab/ui/controller.go`
- Modify: `internal/aphelion/collab/ui/controller_test.go`
- Modify: `internal/aphelion/collab/ui/invitation.go`
- Modify: `internal/aphelion/collab/ui/invitation_test.go`
- Modify: `internal/aphelion/collab/ui/session_client.go`
- Modify: `internal/aphelion/collab/ui/viewmodel.go`
- Modify: `internal/aphelion/collab/ui/viewmodel_test.go`
- Modify: `internal/aphelion/collab/ui/panel.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/action_user.go`
- Modify: `internal/app/ui/menu/menu.go`
- Modify: `internal/app/ui/cpwsarea/wsmap/pmap/editor/collaboration.go`
- Modify: `internal/aphelion/collab/ui/defaults_test.go`
- Modify: `internal/aphelion/collab/ui/controller_test.go`
- Modify: `internal/aphelion/collab/ui/invitation_test.go`
- Modify: `internal/aphelion/collab/ui/session_client_test.go`
- Modify: `internal/aphelion/collab/ui/viewmodel_test.go`
- Create: `internal/aphelion/collab/ui/panel_test.go`
- Create: `internal/app/collaboration_relay_test.go`
- Create: `internal/app/ui/menu/collaboration_test.go`
- Modify: `internal/app/ui/cpwsarea/wsmap/pmap/editor/collaboration_test.go`

**Interfaces:**
- Consumes: identity manager, SQLite client store, relay client, owner/replica executors, and existing editor attachment seam.
- Produces: start-online-owner, join-v2, leave/pause/reconnect/profile/invite actions and persisted relay preference.

- [ ] **Step 1: Add a versioned collaboration preference**

```go
type collaborationConfig struct {
	Version  uint
	RelayURL string
}
```

Default to `https://mapping.a13.info`. Preserve a user-configured URL across restart. Reject cleartext non-loopback values and require confirmation when an invitation endpoint differs from the configured endpoint.

- [ ] **Step 2: Initialize the local database and identity during app startup**

Use the existing `internalDir` to derive neutral application-data paths without logging secret/database paths. If initialization fails, keep single-user editing available and show online collaboration as unavailable with a bounded diagnostic.

- [ ] **Step 3: Replace hosted sign-in menu flow with relay session flow**

Remove “Sign In to Hosted Service” and “Sign Out” from the default v2 UI. Add “Start Online Session”, “Join Session”, “Collaboration Settings”, and retain “Start Local Session” during migration. Starting online asks for display name and relay URL, creates the local owner session, connects the relay, then attaches the executor on the UI thread after editor revalidation.

- [ ] **Step 4: Upgrade invitation encode/parse and endpoint confirmation**

Accept `apheliondmm://join` v2 values and keep v1 JSON parsing behind an explicitly labeled legacy path. Clear invite text immediately after parsing and never log or include it in errors.

- [ ] **Step 5: Expose owner-offline, desync, recovery, and pending-review state**

Add view-model fields `Paused`, `Desynchronized`, `RecoveryAction`, and pending counts by ready/conflicting/obsolete. Disable mutation while paused/desynchronized and keep inspect/export/leave available.

- [ ] **Step 6: Fix profile behavior for every role**

Owner and participant name updates use the same signed `ProfileUpdate` flow. Update the panel field after acknowledgement, not merely button press. Test owner rename propagation, participant persistence, invalid/oversized names, and stale profile sequence rejection.

- [ ] **Step 7: Add the online owner-transfer action**

Expose transfer only to the current owner and only for a caught-up editor. Require a confirmation naming the target, run the dual-signed handoff, and change both clients' roles only after the relay and local stores acknowledge the new manifest. Any disconnect or rejection leaves the old owner authoritative.

- [ ] **Step 8: Preserve editor safety and legacy behavior**

Keep the existing executor attachment/project replacement guards. Local single-user and embedded v1 smoke paths must still work while v2 is being piloted.

- [ ] **Step 9: Run UI/controller/editor tests and Windows build**

Run: `go test ./internal/aphelion/collab/ui ./internal/app/... -count=1` then `task build-editor`. Inspect `$LASTEXITCODE` after each native build command.

- [ ] **Step 10: Review checkpoint**

Confirm the UI never exposes keys/capabilities, owner rename is enabled, and endpoint changes are explicit. Leave uncommitted.

---

### Task 10: Relay smoke, load, restart, and adverse-network verification

**Files:**
- Create: `internal/aphelion/smoke/relay.go`
- Create: `internal/aphelion/smoke/relay_test.go`
- Create: `internal/aphelion/collab/load/relay_runner.go`
- Create: `internal/aphelion/collab/load/relay_runner_test.go`
- Create: `internal/aphelion/collab/load/relay_scenario.go`
- Create: `internal/aphelion/collab/load/relay_scenario_test.go`
- Modify: `cmd/apheliondmm-smoke/main.go`
- Modify: `cmd/apheliondmm-smoke/main_test.go`
- Modify: `cmd/apheliondmm-loadtest/main.go`
- Modify: `cmd/apheliondmm-loadtest/main_test.go`

**Interfaces:**
- Consumes: real relay command/service, owner/participant clients, locally minted invitation capabilities, and existing report patterns.
- Produces: relay smoke report, hosted-compatible load credentials supplied by an owner fixture, and bounded restart/adverse-network scenarios.

- [ ] **Step 1: Write a shipped-entry-point smoke test**

Launch `apheliondmm-relay` with a temporary YAML file, connect owner/editor, perform edit/inverse/profile update, restart the relay process, reconnect, and assert identical final hashes plus clean shutdown.

- [ ] **Step 2: Replace server token minting in the v2 load path**

The load runner must consume an owner-generated test bundle containing relay URL, room ID, owner fixture endpoint/key, and one-use admissions. It must never call a relay endpoint to mint identities or invitations.

- [ ] **Step 3: Add load scenarios**

Cover 2, 8, and 32 participants; presence pressure; durable bursts; slow consumer; owner disconnect/reconnect; relay restart; invalid signature; expired/replayed invitation; and a deliberate desync that recovers by replay then snapshot.

- [ ] **Step 4: Add privacy assertions**

Scan captured relay logs and metrics for fixture plaintext, map paths, display names, raw admissions, group keys, and private keys. The expected count is zero.

- [ ] **Step 5: Run repeat and race gates**

Run: `go test -race ./internal/aphelion/smoke ./internal/aphelion/collab/load ./cmd/apheliondmm-smoke ./cmd/apheliondmm-loadtest -count=1` and repeat the relay restart test 20 times.

- [ ] **Step 6: Review checkpoint**

Record exact command output in a new verification document only after the commands run; do not predeclare success. Leave uncommitted.

---

### Task 11: Protected relay container, one-file operations path, and CI gates

**Protected change gate:** Before editing, present this exact set and effect for explicit confirmation:

- Create `deploy/relay/Dockerfile`: build/run only `apheliondmm-relay` and the healthcheck as nonroot.
- Create `deploy/relay/compose.yaml`, `compose.cloudflare.yaml`, and `compose.loopback.yaml`: one relay service plus optional public Cloudflare sidecar; no PostgreSQL/OIDC/volumes.
- Create `deploy/relay/relay.yaml.example` and `.env.example`: the single non-secret configuration and image metadata.
- Create `deploy/relay/operations.ps1`: `Setup`, `Validate`, `Build`, `StartCloudflare`, `StartLoopback`, `Status`, `Logs`, `Update`, and `Stop`; no backup/restore/migration actions.
- Create `deploy/relay/README.md`: public and self-hosted operator guide.
- Modify `.github/workflows/ci.yml`: add relay race, command, config/schema, image, and container-lifecycle gates; keep v1 gates during migration.
- Modify `Taskfile.yml`: add relay build/test tasks without changing existing default editor build behavior.
- Create `internal/aphelion/collab/container/relay_config_test.go` and `relay_integration_test.go`: validate Compose and real container lifecycle.

**Interfaces:**
- Consumes: relay command/config/healthcheck and existing pinned-image/security patterns.
- Produces: reproducible separate relay stack suitable for `mapping.a13.info` and self-hosting.

- [x] **Step 1: Obtain protected-file confirmation**

Do not edit any file in this task until the user explicitly confirms the set above.

- [x] **Step 2: Write container/config tests first**

Assert no `postgres`, OIDC, database secret, backup volume, or map mount exists; relay binds to the private network; root filesystem is read-only; capabilities are dropped; healthcheck passes; restart loses rooms; and one YAML file controls the relay.

- [x] **Step 3: Create the minimal distroless relay image**

Pin base image digests, build with `CGO_ENABLED=0`, run nonroot, expose 8080, and use the existing healthcheck command against relay readiness.

- [x] **Step 4: Create Compose overlays**

Cloudflare overlay reads only `secrets/cloudflare_tunnel_token.txt`. Loopback overlay publishes `127.0.0.1:8080:8080`. Neither path opens a database network or volume.

- [x] **Step 5: Implement the singular PowerShell operator entry point**

`Setup` creates `relay.yaml` and the secrets directory, validates Docker, rejects symlinks, and prints the required public mapping `mapping.a13.info -> http://relay:8080`. `Validate` runs schema and Compose checks. `Update` builds/pulls, replaces only the relay, waits for health, and preserves the prior image tag for manual rollback.

- [x] **Step 6: Add CI and Task gates**

Add exact commands for protocol-v2 fuzz decoding, race tests, relay container build, Compose validation, and real container lifecycle. Do not remove v1 PostgreSQL/OIDC gates before acceptance.

- [x] **Step 7: Run local container verification**

Run the operations script for `Setup`, `Validate`, `Build`, `StartLoopback`, `Status`, a healthcheck, relay restart, and `Stop`. Capture Docker versions and image digest.

- [x] **Step 8: Review checkpoint**

Confirm an uninformed self-hoster needs only Docker, `relay.yaml`, and an optional tunnel-token file. Leave uncommitted.

---

### Task 12: Agent guidance, self-hosting handoff, human test guide, and legacy labeling

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/agent/architecture.md`
- Modify: `docs/agent/multiplayer-invariants.md`
- Modify: `docs/agent/security-and-networking.md`
- Modify: `docs/agent/verification.md`
- Modify: `docs/superpowers/specs/2026-08-24-multiplayer-design.md`
- Modify: `docs/hosting/game-server-deployment-agent-handoff.md`
- Create: `docs/hosting/client-owned-relay-agent-handoff.md`
- Create: `docs/testing/client-owned-relay-human-test-guide.md`
- Modify: `docs/superpowers/plans/2026-08-26-final-multiplayer-progression-sheet.md`
- Modify: `docs/superpowers/plans/README.md`

**Interfaces:**
- Consumes: verified commands/artifacts from Tasks 1-11.
- Produces: current authority rules, repeatable operator handoff, honest progression sheet, and human pilot instructions.

- [x] **Step 1: Update authority and security guidance**

State that protocol v2 is owner-client authoritative, client-persistent, relay-opaque, and paused without the owner. Retain protocol-v1 rules under a clearly labeled legacy section so agents do not apply the wrong invariants.

- [x] **Step 2: Mark the 2026-08-24 design as historically superseded for online v2**

Add a status notice linking the approved v2 spec. Do not rewrite historical decisions or delete v1 documentation.

- [x] **Step 3: Write the relay agent/operator handoff**

Document file roles, one-file configuration, secret handling, official hostname, public/no-Access requirement, build/start/update/status/log/stop commands, health checks, safe rollback, and the fact that relay restarts need no database restore.

- [x] **Step 4: Rewrite the game-server handoff boundary**

State that the game server only hosts the relay container/tunnel. Remove instructions that ask it to operate PostgreSQL/OIDC for v2; preserve a link to legacy v1 procedures.

- [x] **Step 5: Write the human test guide**

Use plain steps for two computers on separate networks. Ask testers to report exact time, client role/version, relay endpoint, revision/hash, action, expected/actual result, recovery behavior, and sanitized logs. Include owner rename, editor rename, online owner transfer, owner-offline pause, relay restart, conflict/undo, desync recovery, save/reopen fidelity, endpoint override, narrow UI, keyboard access, and privacy checks.

- [x] **Step 6: Update progression accounting honestly**

Separate implemented/automated, locally exercised, public-online tested, and accepted. Do not call the project complete until the public two-network human pilot passes.

- [x] **Step 7: Run documentation and privacy checks**

Run `git diff --check`, tracked personal-name/profile-path scans, YAML/JSON schema validation, and link/path existence checks. Generated caches remain ignored and are not published.

- [x] **Step 8: Review checkpoint**

Confirm Content Tools is absent and no document embeds a real name, account name, home path, secret, or invitation. Leave uncommitted.

---

### Task 13: Full automated acceptance and handoff to human online testing

**Files:**
- Create: `docs/verification/client-owned-relay-automated-2026-08-26.md`
- Modify only failing implementation/tests discovered by the gates above

**Interfaces:**
- Consumes: all Tasks 1-12.
- Produces: evidence-backed readiness report and exact remaining human-only checklist.

- [x] **Step 1: Run formatting and focused package gates**

Run `gofmt` on changed Go files, protocol-v2 tests, identity/store tests, authority/replica tests, relay/client tests, UI/app tests, smoke/load tests, and container configuration tests. Record exact commands and exit codes.

- [x] **Step 2: Run repository-wide Go and race gates**

Run: `go test ./... -count=1` and `go test -race ./internal/aphelion/... -count=1`.

- [x] **Step 3: Run Rust/parser and shipped Windows build gates**

Run: `task test-rust` and `task build`. Inspect `$LASTEXITCODE`, produced executable existence, and SHA-256. Do not treat helper-only tests as shipped-entry-point proof.

- [x] **Step 4: Run real relay container lifecycle and restart smoke**

Use the exact built image/config, loopback Compose overlay, two real clients, and a relay-only restart. Confirm accepted revision/hash survives exclusively through clients.

- [x] **Step 5: Run dependency, vulnerability, and privacy checks**

Run the repository's existing `govulncheck`, dependency policy, tracked privacy scan, secret scan, and container scan gates. Report inherited warnings separately from new failures.

- [x] **Step 6: Write the evidence document**

Include revision/working-tree state, tool versions, commands, results, image digest, executable hash, known limitations, and the remaining public human-test items. Do not state that `mapping.a13.info` is deployed or tested unless it was actually exercised.

- [x] **Step 7: Stop at the human-test gate**

Provide the tester guide and request the public two-network pilot. Do not switch `deploy/production` from v1, delete PostgreSQL/OIDC code, or claim multiplayer completion before that result.

- [x] **Step 8: Review checkpoint**

Leave all changes visible in the working tree. Offer a commit/branch/push only if the user explicitly requests it.

---

## Post-pilot cutover (separate approval and plan)

After the public pilot passes, prepare a small protected cutover plan to make protocol v2 the default online UI/deployment, move v1 PostgreSQL/OIDC deployment material under a labeled legacy path, update release packaging, and decide whether to retain or remove v1 code. That cutover is intentionally excluded here because its acceptance evidence does not exist yet.
