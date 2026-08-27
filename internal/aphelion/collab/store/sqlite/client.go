package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
	collabstore "sdmm/internal/aphelion/collab/store"
)

func (store *Store) CreateClientSession(ctx context.Context, session collabstore.LocalSession) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	if err := validateLocalSession(session); err != nil {
		return err
	}
	expectedHash, found, err := revisionHash(ctx, store.database, session.DocumentID, session.AcknowledgedRevision)
	if err != nil {
		return err
	}
	if !found || expectedHash != session.AcknowledgedMapHash {
		return fmt.Errorf("client session acknowledgement does not match stored document")
	}
	_, err = store.database.ExecContext(ctx, `INSERT INTO client_sessions(
		session_id, relay_url, role, document_id, owner_public_key, manifest, manifest_sha256,
		acknowledged_revision, acknowledged_map_hash, display_name, identity_secret_ref,
		group_key_secret_ref, owner_secret_ref
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.SessionID, session.RelayURL, session.Role, session.DocumentID, session.OwnerPublicKey[:],
		session.Manifest, session.ManifestSHA256[:], session.AcknowledgedRevision, session.AcknowledgedMapHash,
		session.DisplayName, session.IdentitySecretRef, session.GroupKeySecretRef, session.OwnerSecretRef)
	if err != nil {
		return fmt.Errorf("create client session: %w", err)
	}
	return nil
}

func (store *Store) LoadClientSession(ctx context.Context, sessionID string) (collabstore.LocalSession, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.LocalSession{}, collabstore.ErrStoreClosed
	}
	return loadClientSession(ctx, store.database, sessionID)
}

func (store *Store) SavePending(ctx context.Context, pending collabstore.PendingSubmission) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	if pending.SessionID == "" || !validDisposition(pending.Disposition) || pending.CreatedAt.IsZero() {
		return fmt.Errorf("pending submission is invalid")
	}
	if err := pending.Operation.OperationID.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(pending.Operation)
	if err != nil {
		return fmt.Errorf("encode pending operation: %w", err)
	}
	_, err = store.database.ExecContext(ctx, `INSERT INTO pending_submissions(session_id, operation_id, operation, disposition, created_at, diagnostic)
		VALUES(?, ?, ?, ?, ?, ?)`, pending.SessionID, pending.Operation.OperationID, encoded, pending.Disposition, pending.CreatedAt.UTC().Format(time.RFC3339Nano), pending.Diagnostic)
	if err != nil {
		return fmt.Errorf("save pending submission: %w", err)
	}
	return nil
}

func (store *Store) ListPending(ctx context.Context, sessionID string) ([]collabstore.PendingSubmission, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return nil, collabstore.ErrStoreClosed
	}
	rows, err := store.database.QueryContext(ctx, `SELECT operation, disposition, created_at, diagnostic
		FROM pending_submissions WHERE session_id = ? ORDER BY created_at, operation_id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list pending submissions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]collabstore.PendingSubmission, 0)
	for rows.Next() {
		var encoded []byte
		var disposition collabstore.PendingDisposition
		var createdAt, diagnostic string
		if err := rows.Scan(&encoded, &disposition, &createdAt, &diagnostic); err != nil {
			return nil, fmt.Errorf("scan pending submission: %w", err)
		}
		var operation model.Operation
		if err := json.Unmarshal(encoded, &operation); err != nil {
			return nil, fmt.Errorf("decode pending operation: %w", err)
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("decode pending timestamp: %w", err)
		}
		items = append(items, collabstore.PendingSubmission{SessionID: sessionID, Operation: operation, Disposition: disposition, CreatedAt: created, Diagnostic: diagnostic})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending submissions: %w", err)
	}
	return items, nil
}

func (store *Store) ResolvePending(ctx context.Context, sessionID string, operationID model.OperationID, disposition collabstore.PendingDisposition, diagnostic string) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	if !validDisposition(disposition) {
		return fmt.Errorf("pending disposition is invalid")
	}
	result, err := store.database.ExecContext(ctx, `UPDATE pending_submissions SET disposition = ?, diagnostic = ?
		WHERE session_id = ? AND operation_id = ?`, disposition, diagnostic, sessionID, operationID)
	if err != nil {
		return fmt.Errorf("resolve pending submission: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read pending resolution result: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("pending submission does not exist")
	}
	return nil
}

func (store *Store) ApplyAccepted(ctx context.Context, sessionID string, accepted model.AcceptedOperation, expectedMapHash string) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin accepted replica operation: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	var documentID model.DocumentID
	if err := transaction.QueryRowContext(ctx, "SELECT document_id FROM client_sessions WHERE session_id = ?", sessionID).Scan(&documentID); err != nil {
		return fmt.Errorf("load accepted replica session: %w", err)
	}
	if documentID != accepted.DocumentID {
		return fmt.Errorf("accepted operation document does not match client session")
	}
	var priorData []byte
	err = transaction.QueryRowContext(ctx, "SELECT accepted FROM operations WHERE document_id = ? AND operation_id = ?", documentID, accepted.OperationID).Scan(&priorData)
	if err == nil {
		prior, decodeErr := decodeAccepted(priorData)
		if decodeErr != nil {
			return decodeErr
		}
		if !reflect.DeepEqual(prior, accepted) {
			return fmt.Errorf("accepted replica operation conflicts with stored operation")
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lookup accepted replica operation: %w", err)
	} else {
		snapshot, replay, err := load(ctx, transaction, documentID)
		if err != nil {
			return err
		}
		document, err := replayDocument(snapshot, replay)
		if err != nil {
			return err
		}
		verified, err := document.Apply(accepted.Operation, accepted.AcceptedAt)
		if err != nil {
			return fmt.Errorf("verify accepted replica operation: %w", err)
		}
		if !reflect.DeepEqual(verified, accepted) {
			return fmt.Errorf("accepted replica operation differs from verified operation")
		}
		actualHash, err := document.Snapshot().Hash()
		if err != nil {
			return err
		}
		if actualHash != expectedMapHash {
			return fmt.Errorf("accepted replica map hash does not match verified state")
		}
		encoded, err := json.Marshal(accepted)
		if err != nil {
			return fmt.Errorf("encode accepted replica operation: %w", err)
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO operations(document_id, operation_id, revision, accepted, map_hash) VALUES(?, ?, ?, ?, ?)", documentID, accepted.OperationID, accepted.Revision, encoded, actualHash); err != nil {
			return fmt.Errorf("insert accepted replica operation: %w", err)
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO revision_hashes(document_id, revision, map_hash) VALUES(?, ?, ?)", documentID, accepted.Revision, actualHash); err != nil {
			return fmt.Errorf("insert accepted replica hash: %w", err)
		}
	}
	result, err := transaction.ExecContext(ctx, "UPDATE client_sessions SET acknowledged_revision = ?, acknowledged_map_hash = ? WHERE session_id = ?", accepted.Revision, expectedMapHash, sessionID)
	if err != nil {
		return fmt.Errorf("update accepted replica acknowledgement: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return fmt.Errorf("client session does not exist")
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM pending_submissions WHERE session_id = ? AND operation_id = ?", sessionID, accepted.OperationID); err != nil {
		return fmt.Errorf("clear accepted pending submission: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit accepted replica operation: %w", err)
	}
	return nil
}

func (store *Store) SaveManifest(ctx context.Context, sessionID string, manifest []byte, admissions []collabstore.Admission) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	if len(manifest) == 0 {
		return fmt.Errorf("manifest is empty")
	}
	for _, admission := range admissions {
		if admission.SessionID != sessionID || (admission.Role != protocolv2.RoleViewer && admission.Role != protocolv2.RoleEditor) || admission.ExpiresAt.IsZero() {
			return fmt.Errorf("admission is invalid")
		}
	}
	digest := sha256.Sum256(manifest)
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin manifest replacement: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	result, err := transaction.ExecContext(ctx, "UPDATE client_sessions SET manifest = ?, manifest_sha256 = ? WHERE session_id = ?", manifest, digest[:], sessionID)
	if err != nil {
		return fmt.Errorf("update manifest: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return fmt.Errorf("client session does not exist")
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM admissions WHERE session_id = ?", sessionID); err != nil {
		return fmt.Errorf("clear admissions: %w", err)
	}
	for _, admission := range admissions {
		if _, err := transaction.ExecContext(ctx, `INSERT INTO admissions(session_id, capability_id, role, expires_at, bound_actor)
			VALUES(?, ?, ?, ?, ?)`, sessionID, admission.CapabilityID[:], admission.Role, admission.ExpiresAt.UTC().Format(time.RFC3339Nano), admission.BoundActor[:]); err != nil {
			return fmt.Errorf("insert admission: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit manifest replacement: %w", err)
	}
	return nil
}

func (store *Store) ListAdmissions(ctx context.Context, sessionID string) ([]collabstore.Admission, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return nil, collabstore.ErrStoreClosed
	}
	rows, err := store.database.QueryContext(ctx, `SELECT capability_id, role, expires_at, bound_actor
		FROM admissions WHERE session_id = ? ORDER BY capability_id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list admissions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]collabstore.Admission, 0)
	for rows.Next() {
		var capability, actor []byte
		var role protocolv2.Role
		var expiresAt string
		if err := rows.Scan(&capability, &role, &expiresAt, &actor); err != nil {
			return nil, fmt.Errorf("scan admission: %w", err)
		}
		if len(capability) != 32 || len(actor) != 32 {
			return nil, fmt.Errorf("admission key length is invalid")
		}
		expires, err := time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil {
			return nil, fmt.Errorf("decode admission expiry: %w", err)
		}
		item := collabstore.Admission{SessionID: sessionID, Role: role, ExpiresAt: expires}
		copy(item.CapabilityID[:], capability)
		copy(item.BoundActor[:], actor)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (store *Store) ReplaceReplica(ctx context.Context, session collabstore.LocalSession, snapshot model.Snapshot, operations []model.AcceptedOperation) error {
	snapshotData, operationData, revisionHashes, finalRevision, finalHash, err := prepareReplica(snapshot, operations)
	if err != nil {
		return err
	}
	if session.DocumentID != snapshot.DocumentID || session.AcknowledgedRevision != finalRevision || session.AcknowledgedMapHash != finalHash {
		return fmt.Errorf("replica acknowledgement does not match verified state")
	}
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replica replacement: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	var documentID model.DocumentID
	if err := transaction.QueryRowContext(ctx, "SELECT document_id FROM client_sessions WHERE session_id = ?", session.SessionID).Scan(&documentID); err != nil {
		return fmt.Errorf("load replica session: %w", err)
	}
	if documentID != snapshot.DocumentID {
		return fmt.Errorf("replica document does not match client session")
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM operations WHERE document_id = ?", documentID); err != nil {
		return fmt.Errorf("clear replica operations: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM revision_hashes WHERE document_id = ?", documentID); err != nil {
		return fmt.Errorf("clear replica hashes: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `UPDATE documents SET snapshot = ?, snapshot_revision = ?, snapshot_hash = ? WHERE document_id = ?`, snapshotData, snapshot.Revision, revisionHashes[0], documentID); err != nil {
		return fmt.Errorf("replace replica snapshot: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, "INSERT INTO revision_hashes(document_id, revision, map_hash) VALUES(?, ?, ?)", documentID, snapshot.Revision, revisionHashes[0]); err != nil {
		return fmt.Errorf("insert replica snapshot hash: %w", err)
	}
	for index, accepted := range operations {
		if _, err := transaction.ExecContext(ctx, `INSERT INTO operations(document_id, operation_id, revision, accepted, map_hash) VALUES(?, ?, ?, ?, ?)`, documentID, accepted.OperationID, accepted.Revision, operationData[index], revisionHashes[index+1]); err != nil {
			return fmt.Errorf("insert replica operation: %w", err)
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO revision_hashes(document_id, revision, map_hash) VALUES(?, ?, ?)", documentID, accepted.Revision, revisionHashes[index+1]); err != nil {
			return fmt.Errorf("insert replica revision hash: %w", err)
		}
	}
	if _, err := transaction.ExecContext(ctx, `UPDATE client_sessions SET acknowledged_revision = ?, acknowledged_map_hash = ? WHERE session_id = ?`, finalRevision, finalHash, session.SessionID); err != nil {
		return fmt.Errorf("update replica acknowledgement: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit replica replacement: %w", err)
	}
	return nil
}

func validateLocalSession(session collabstore.LocalSession) error {
	if session.SessionID == "" || session.RelayURL == "" || session.DocumentID == "" || session.DisplayName == "" {
		return fmt.Errorf("client session is incomplete")
	}
	if session.Role != protocolv2.RoleViewer && session.Role != protocolv2.RoleEditor && session.Role != protocolv2.RoleOwner {
		return fmt.Errorf("client session role is invalid")
	}
	if len(session.Manifest) == 0 || sha256.Sum256(session.Manifest) != session.ManifestSHA256 {
		return fmt.Errorf("client session manifest digest is invalid")
	}
	if session.OwnerPublicKey == (protocolv2.ActorKey{}) || session.IdentitySecretRef == "" || session.GroupKeySecretRef == "" {
		return fmt.Errorf("client session key references are incomplete")
	}
	return nil
}

func loadClientSession(ctx context.Context, database queryer, sessionID string) (collabstore.LocalSession, error) {
	var session collabstore.LocalSession
	var ownerKey, digest []byte
	err := database.QueryRowContext(ctx, `SELECT session_id, relay_url, role, document_id, owner_public_key, manifest,
		manifest_sha256, acknowledged_revision, acknowledged_map_hash, display_name, identity_secret_ref,
		group_key_secret_ref, owner_secret_ref FROM client_sessions WHERE session_id = ?`, sessionID).Scan(
		&session.SessionID, &session.RelayURL, &session.Role, &session.DocumentID, &ownerKey, &session.Manifest,
		&digest, &session.AcknowledgedRevision, &session.AcknowledgedMapHash, &session.DisplayName,
		&session.IdentitySecretRef, &session.GroupKeySecretRef, &session.OwnerSecretRef)
	if errors.Is(err, sql.ErrNoRows) {
		return collabstore.LocalSession{}, collabstore.ErrSessionMissing
	}
	if err != nil {
		return collabstore.LocalSession{}, fmt.Errorf("load client session: %w", err)
	}
	if len(ownerKey) != 32 || len(digest) != 32 {
		return collabstore.LocalSession{}, fmt.Errorf("client session key or digest length is invalid")
	}
	copy(session.OwnerPublicKey[:], ownerKey)
	copy(session.ManifestSHA256[:], digest)
	if err := validateLocalSession(session); err != nil {
		return collabstore.LocalSession{}, err
	}
	return session, nil
}

func prepareReplica(snapshot model.Snapshot, operations []model.AcceptedOperation) ([]byte, [][]byte, []string, model.Revision, string, error) {
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		return nil, nil, nil, 0, "", fmt.Errorf("verify replica snapshot: %w", err)
	}
	snapshotData, err := json.Marshal(snapshot)
	if err != nil {
		return nil, nil, nil, 0, "", fmt.Errorf("encode replica snapshot: %w", err)
	}
	initialHash, err := snapshot.Hash()
	if err != nil {
		return nil, nil, nil, 0, "", err
	}
	hashes := []string{initialHash}
	encoded := make([][]byte, 0, len(operations))
	for _, accepted := range operations {
		verified, err := document.Apply(accepted.Operation, accepted.AcceptedAt)
		if err != nil {
			return nil, nil, nil, 0, "", fmt.Errorf("verify replica revision %d: %w", accepted.Revision, err)
		}
		if !reflect.DeepEqual(verified, accepted) {
			return nil, nil, nil, 0, "", fmt.Errorf("replica revision %d differs from accepted operation", accepted.Revision)
		}
		data, err := json.Marshal(accepted)
		if err != nil {
			return nil, nil, nil, 0, "", fmt.Errorf("encode replica revision %d: %w", accepted.Revision, err)
		}
		hash, err := document.Snapshot().Hash()
		if err != nil {
			return nil, nil, nil, 0, "", err
		}
		encoded = append(encoded, data)
		hashes = append(hashes, hash)
	}
	state := document.Snapshot()
	return snapshotData, encoded, hashes, state.Revision, hashes[len(hashes)-1], nil
}

func validDisposition(disposition collabstore.PendingDisposition) bool {
	switch disposition {
	case collabstore.PendingReady, collabstore.PendingConflicting, collabstore.PendingObsolete, collabstore.PendingSubmitted:
		return true
	default:
		return false
	}
}

func revisionHash(ctx context.Context, database queryer, documentID model.DocumentID, revision model.Revision) (string, bool, error) {
	var hash string
	err := database.QueryRowContext(ctx, "SELECT map_hash FROM revision_hashes WHERE document_id = ? AND revision = ?", documentID, revision).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("query client revision hash: %w", err)
	}
	return hash, true, nil
}
