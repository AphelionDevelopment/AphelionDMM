package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	collabstore "sdmm/internal/aphelion/collab/store"
)

const (
	defaultMaxConnections   = 8
	defaultStatementTimeout = 5 * time.Second
	maxTransactionAttempts  = 4
)

var postgresIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Config struct {
	DSN              string
	MaxConnections   int32
	MinConnections   int32
	ConnectTimeout   time.Duration
	StatementTimeout time.Duration
	Schema           string
}

type Store struct {
	mutex  sync.RWMutex
	pool   *pgxpool.Pool
	closed bool
}

func Open(ctx context.Context, config Config) (*Store, error) {
	if config.DSN == "" {
		return nil, fmt.Errorf("open PostgreSQL store: DSN is empty")
	}
	poolConfig, err := pgxpool.ParseConfig(config.DSN)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL store: parse DSN: %w", err)
	}
	if config.MaxConnections <= 0 {
		config.MaxConnections = defaultMaxConnections
	}
	if config.MinConnections < 0 || config.MinConnections > config.MaxConnections {
		return nil, fmt.Errorf("open PostgreSQL store: connection bounds are invalid")
	}
	if config.StatementTimeout <= 0 {
		config.StatementTimeout = defaultStatementTimeout
	}
	poolConfig.MaxConns = config.MaxConnections
	poolConfig.MinConns = config.MinConnections
	if config.Schema != "" {
		if !postgresIdentifierPattern.MatchString(config.Schema) {
			return nil, fmt.Errorf("open PostgreSQL store: schema name is invalid")
		}
		poolConfig.ConnConfig.RuntimeParams["search_path"] = config.Schema
	}
	if config.ConnectTimeout > 0 {
		poolConfig.ConnConfig.ConnectTimeout = config.ConnectTimeout
	}
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", config.StatementTimeout.Milliseconds())
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL store: configure pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("open PostgreSQL store: connect: %w", err)
	}
	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (store *Store) Ready(ctx context.Context) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	if err := store.pool.Ping(ctx); err != nil {
		return fmt.Errorf("check PostgreSQL readiness: %w", err)
	}
	return nil
}

func (store *Store) Create(ctx context.Context, snapshot model.Snapshot) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	mapHash, err := snapshot.Hash()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode PostgreSQL snapshot: %w", err)
	}
	transaction, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin PostgreSQL create: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	result, err := transaction.Exec(ctx, `INSERT INTO collaboration_documents(document_id, snapshot, snapshot_revision, snapshot_hash, current_revision, current_hash) VALUES($1, $2, $3, $4, $3, $4) ON CONFLICT (document_id) DO NOTHING`, snapshot.DocumentID, encoded, snapshot.Revision, mapHash)
	if err != nil {
		return fmt.Errorf("create PostgreSQL session: %w", err)
	}
	if result.RowsAffected() != 1 {
		return collabstore.ErrSessionExists
	}
	if _, err := transaction.Exec(ctx, `INSERT INTO collaboration_revision_hashes(document_id, revision, map_hash) VALUES($1, $2, $3)`, snapshot.DocumentID, snapshot.Revision, mapHash); err != nil {
		return fmt.Errorf("create PostgreSQL revision hash: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL create: %w", err)
	}
	return nil
}

func (store *Store) Append(ctx context.Context, accepted model.AcceptedOperation) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	var err error
	for attempt := 0; attempt < maxTransactionAttempts; attempt++ {
		err = store.appendOnce(ctx, accepted)
		if err == nil || !IsRetryable(err) {
			return err
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
	}
	return fmt.Errorf("append PostgreSQL operation after %d attempts: %w", maxTransactionAttempts, err)
}

func (store *Store) appendOnce(ctx context.Context, accepted model.AcceptedOperation) error {
	transaction, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin PostgreSQL append: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	prior, found, err := lookupOperation(ctx, transaction, accepted.DocumentID, accepted.OperationID)
	if err != nil {
		return err
	}
	if found {
		if reflect.DeepEqual(prior, accepted) {
			return nil
		}
		return fmt.Errorf("operation %q conflicts with stored revision %d", accepted.OperationID, prior.Revision)
	}
	base, replay, currentRevision, err := load(ctx, transaction, accepted.DocumentID, true)
	if err != nil {
		return err
	}
	if accepted.Revision != currentRevision+1 {
		return fmt.Errorf("accepted revision %d is not contiguous after %d", accepted.Revision, currentRevision)
	}
	document, err := replayDocument(base, replay)
	if err != nil {
		return err
	}
	verified, err := document.Apply(accepted.Operation, accepted.AcceptedAt)
	if err != nil {
		return fmt.Errorf("verify PostgreSQL accepted operation: %w", err)
	}
	if !reflect.DeepEqual(verified, accepted) {
		return fmt.Errorf("accepted operation does not match verified operation")
	}
	mapHash, err := document.Snapshot().Hash()
	if err != nil {
		return fmt.Errorf("hash PostgreSQL accepted revision: %w", err)
	}
	encoded, err := json.Marshal(accepted)
	if err != nil {
		return fmt.Errorf("encode PostgreSQL accepted operation: %w", err)
	}
	if _, err := transaction.Exec(ctx, `INSERT INTO collaboration_operations(document_id, operation_id, revision, accepted, map_hash) VALUES($1, $2, $3, $4, $5)`, accepted.DocumentID, accepted.OperationID, accepted.Revision, encoded, mapHash); err != nil {
		return fmt.Errorf("insert PostgreSQL operation: %w", err)
	}
	if _, err := transaction.Exec(ctx, `INSERT INTO collaboration_revision_hashes(document_id, revision, map_hash) VALUES($1, $2, $3)`, accepted.DocumentID, accepted.Revision, mapHash); err != nil {
		return fmt.Errorf("insert PostgreSQL revision hash: %w", err)
	}
	if _, err := transaction.Exec(ctx, `UPDATE collaboration_documents SET current_revision = $2, current_hash = $3 WHERE document_id = $1`, accepted.DocumentID, accepted.Revision, mapHash); err != nil {
		return fmt.Errorf("advance PostgreSQL document revision: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL append: %w", err)
	}
	return nil
}

func (store *Store) Load(ctx context.Context, documentID model.DocumentID) (model.Snapshot, []model.AcceptedOperation, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return model.Snapshot{}, nil, collabstore.ErrStoreClosed
	}
	base, replay, _, err := load(ctx, store.pool, documentID, false)
	return base, replay, err
}

func (store *Store) SaveSnapshot(ctx context.Context, snapshot model.Snapshot) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	transaction, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin PostgreSQL snapshot: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	base, replay, _, err := load(ctx, transaction, snapshot.DocumentID, true)
	if err != nil {
		return err
	}
	retained, err := replaySnapshotAt(base, replay, snapshot.Revision)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(retained, model.CloneSnapshot(snapshot)) {
		return fmt.Errorf("snapshot does not match retained revision")
	}
	mapHash, err := snapshot.Hash()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode PostgreSQL snapshot: %w", err)
	}
	result, err := transaction.Exec(ctx, `UPDATE collaboration_documents SET snapshot = $2, snapshot_revision = $3, snapshot_hash = $4 WHERE document_id = $1`, snapshot.DocumentID, encoded, snapshot.Revision, mapHash)
	if err != nil {
		return fmt.Errorf("update PostgreSQL snapshot: %w", err)
	}
	if result.RowsAffected() != 1 {
		return collabstore.ErrSessionMissing
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL snapshot: %w", err)
	}
	return nil
}

func (store *Store) RevisionHash(ctx context.Context, documentID model.DocumentID, revision model.Revision) (string, bool, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return "", false, collabstore.ErrStoreClosed
	}
	var hash string
	err := store.pool.QueryRow(ctx, `SELECT map_hash FROM collaboration_revision_hashes WHERE document_id = $1 AND revision = $2`, documentID, revision).Scan(&hash)
	if err == nil {
		return hash, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("query PostgreSQL revision hash: %w", err)
	}
	if exists, err := documentExists(ctx, store.pool, documentID); err != nil || !exists {
		return "", false, missingOrError(err)
	}
	return "", false, nil
}

func (store *Store) LookupOperation(ctx context.Context, documentID model.DocumentID, operationID model.OperationID) (model.AcceptedOperation, bool, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return model.AcceptedOperation{}, false, collabstore.ErrStoreClosed
	}
	accepted, found, err := lookupOperation(ctx, store.pool, documentID, operationID)
	if err != nil || found {
		return accepted, found, err
	}
	if exists, existsErr := documentExists(ctx, store.pool, documentID); existsErr != nil || !exists {
		return model.AcceptedOperation{}, false, missingOrError(existsErr)
	}
	return model.AcceptedOperation{}, false, nil
}

func (store *Store) CreateExportCheckpoint(ctx context.Context, checkpoint model.ExportCheckpoint) (model.ExportCheckpoint, bool, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return model.ExportCheckpoint{}, false, collabstore.ErrStoreClosed
	}
	if err := checkpoint.Validate(); err != nil {
		return model.ExportCheckpoint{}, false, err
	}
	if checkpoint.Status != model.ExportCheckpointPending {
		return model.ExportCheckpoint{}, false, fmt.Errorf("new export checkpoint must be pending")
	}
	transaction, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return model.ExportCheckpoint{}, false, fmt.Errorf("begin PostgreSQL export checkpoint: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	var present int
	if err := transaction.QueryRow(ctx, `SELECT 1 FROM collaboration_documents WHERE document_id = $1 FOR UPDATE`, checkpoint.DocumentID).Scan(&present); errors.Is(err, pgx.ErrNoRows) {
		return model.ExportCheckpoint{}, false, collabstore.ErrSessionMissing
	} else if err != nil {
		return model.ExportCheckpoint{}, false, fmt.Errorf("lock PostgreSQL checkpoint document: %w", err)
	}
	prior, found, err := lookupExportCheckpointByKey(ctx, transaction, checkpoint.DocumentID, checkpoint.IdempotencyKey)
	if err != nil {
		return model.ExportCheckpoint{}, false, err
	}
	if found {
		if collabstore.SameCheckpointRequest(prior, checkpoint) {
			return prior, false, nil
		}
		return model.ExportCheckpoint{}, false, collabstore.ErrCheckpointConflict
	}
	prior, found, err = lookupExportCheckpoint(ctx, transaction, checkpoint.DocumentID, checkpoint.CheckpointID)
	if err != nil {
		return model.ExportCheckpoint{}, false, err
	}
	if found {
		if collabstore.SameCheckpointRequest(prior, checkpoint) {
			return prior, false, nil
		}
		return model.ExportCheckpoint{}, false, collabstore.ErrCheckpointConflict
	}
	var retainedHash string
	if err := transaction.QueryRow(ctx, `SELECT map_hash FROM collaboration_revision_hashes WHERE document_id = $1 AND revision = $2`, checkpoint.DocumentID, checkpoint.Revision).Scan(&retainedHash); errors.Is(err, pgx.ErrNoRows) {
		return model.ExportCheckpoint{}, false, fmt.Errorf("checkpoint revision/hash is not retained")
	} else if err != nil {
		return model.ExportCheckpoint{}, false, fmt.Errorf("query PostgreSQL checkpoint revision hash: %w", err)
	}
	if retainedHash != checkpoint.MapHash {
		return model.ExportCheckpoint{}, false, fmt.Errorf("checkpoint revision/hash is not retained")
	}
	if _, err := transaction.Exec(ctx, `INSERT INTO collaboration_export_checkpoints(checkpoint_id, document_id, session_id, idempotency_key, revision, map_hash, requested_by, created_at, status) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)`, checkpoint.CheckpointID, checkpoint.DocumentID, checkpoint.SessionID, checkpoint.IdempotencyKey, checkpoint.Revision, checkpoint.MapHash, checkpoint.RequestedBy, checkpoint.CreatedAt, checkpoint.Status); err != nil {
		return model.ExportCheckpoint{}, false, fmt.Errorf("insert PostgreSQL export checkpoint: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return model.ExportCheckpoint{}, false, fmt.Errorf("commit PostgreSQL export checkpoint: %w", err)
	}
	return model.CloneExportCheckpoint(checkpoint), true, nil
}

func (store *Store) LookupExportCheckpoint(ctx context.Context, documentID model.DocumentID, checkpointID model.CheckpointID) (model.ExportCheckpoint, bool, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return model.ExportCheckpoint{}, false, collabstore.ErrStoreClosed
	}
	checkpoint, found, err := lookupExportCheckpoint(ctx, store.pool, documentID, checkpointID)
	if err != nil || found {
		return checkpoint, found, err
	}
	if exists, existsErr := documentExists(ctx, store.pool, documentID); existsErr != nil || !exists {
		return model.ExportCheckpoint{}, false, missingOrError(existsErr)
	}
	return model.ExportCheckpoint{}, false, nil
}

func (store *Store) CompleteExportCheckpoint(ctx context.Context, documentID model.DocumentID, checkpointID model.CheckpointID, completion model.ExportCheckpointCompletion) (model.ExportCheckpoint, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return model.ExportCheckpoint{}, collabstore.ErrStoreClosed
	}
	transaction, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return model.ExportCheckpoint{}, fmt.Errorf("begin PostgreSQL checkpoint completion: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	checkpoint, found, err := lookupExportCheckpointForUpdate(ctx, transaction, documentID, checkpointID)
	if err != nil {
		return model.ExportCheckpoint{}, err
	}
	if !found {
		return model.ExportCheckpoint{}, collabstore.ErrCheckpointMissing
	}
	if checkpoint.Status != model.ExportCheckpointPending {
		if collabstore.CheckpointMatchesCompletion(checkpoint, completion) {
			return checkpoint, nil
		}
		return model.ExportCheckpoint{}, collabstore.ErrCheckpointTerminal
	}
	completed, err := checkpoint.Complete(completion)
	if err != nil {
		return model.ExportCheckpoint{}, err
	}
	var artifactHash, diagnosticCode any
	if completed.ArtifactHash != "" {
		artifactHash = completed.ArtifactHash
	}
	if completed.DiagnosticCode != "" {
		diagnosticCode = completed.DiagnosticCode
	}
	result, err := transaction.Exec(ctx, `UPDATE collaboration_export_checkpoints SET status = $3, artifact_hash = $4, verifier = $5, verifier_version = $6, diagnostic_code = $7, completed_at = $8 WHERE document_id = $1 AND checkpoint_id = $2 AND status = 'pending'`, documentID, checkpointID, completed.Status, artifactHash, completed.Verifier, completed.VerifierVersion, diagnosticCode, completed.CompletedAt)
	if err != nil {
		return model.ExportCheckpoint{}, fmt.Errorf("update PostgreSQL export checkpoint: %w", err)
	}
	if result.RowsAffected() != 1 {
		return model.ExportCheckpoint{}, fmt.Errorf("complete PostgreSQL export checkpoint changed %d rows", result.RowsAffected())
	}
	if err := transaction.Commit(ctx); err != nil {
		return model.ExportCheckpoint{}, fmt.Errorf("commit PostgreSQL checkpoint completion: %w", err)
	}
	return completed, nil
}

func (store *Store) Close() error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.closed {
		return nil
	}
	store.closed = true
	store.pool.Close()
	return nil
}

func IsRetryable(err error) bool {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return false
	}
	switch postgresError.Code {
	case "40001", "40P01", "57P01", "57P02", "57P03":
		return true
	default:
		return len(postgresError.Code) >= 2 && postgresError.Code[:2] == "08"
	}
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

const exportCheckpointColumns = `checkpoint_id, document_id, session_id, idempotency_key, revision, map_hash, requested_by, created_at, status, artifact_hash, verifier, verifier_version, diagnostic_code, completed_at`

func lookupExportCheckpoint(ctx context.Context, database queryer, documentID model.DocumentID, checkpointID model.CheckpointID) (model.ExportCheckpoint, bool, error) {
	return scanExportCheckpoint(database.QueryRow(ctx, `SELECT `+exportCheckpointColumns+` FROM collaboration_export_checkpoints WHERE document_id = $1 AND checkpoint_id = $2`, documentID, checkpointID))
}

func lookupExportCheckpointForUpdate(ctx context.Context, database queryer, documentID model.DocumentID, checkpointID model.CheckpointID) (model.ExportCheckpoint, bool, error) {
	return scanExportCheckpoint(database.QueryRow(ctx, `SELECT `+exportCheckpointColumns+` FROM collaboration_export_checkpoints WHERE document_id = $1 AND checkpoint_id = $2 FOR UPDATE`, documentID, checkpointID))
}

func lookupExportCheckpointByKey(ctx context.Context, database queryer, documentID model.DocumentID, idempotencyKey string) (model.ExportCheckpoint, bool, error) {
	return scanExportCheckpoint(database.QueryRow(ctx, `SELECT `+exportCheckpointColumns+` FROM collaboration_export_checkpoints WHERE document_id = $1 AND idempotency_key = $2`, documentID, idempotencyKey))
}

func scanExportCheckpoint(row pgx.Row) (model.ExportCheckpoint, bool, error) {
	var checkpoint model.ExportCheckpoint
	var artifactHash, verifier, verifierVersion, diagnosticCode *string
	var completedAt *time.Time
	if err := row.Scan(&checkpoint.CheckpointID, &checkpoint.DocumentID, &checkpoint.SessionID, &checkpoint.IdempotencyKey, &checkpoint.Revision, &checkpoint.MapHash, &checkpoint.RequestedBy, &checkpoint.CreatedAt, &checkpoint.Status, &artifactHash, &verifier, &verifierVersion, &diagnosticCode, &completedAt); errors.Is(err, pgx.ErrNoRows) {
		return model.ExportCheckpoint{}, false, nil
	} else if err != nil {
		return model.ExportCheckpoint{}, false, fmt.Errorf("scan PostgreSQL export checkpoint: %w", err)
	}
	if artifactHash != nil {
		checkpoint.ArtifactHash = *artifactHash
	}
	if verifier != nil {
		checkpoint.Verifier = *verifier
	}
	if verifierVersion != nil {
		checkpoint.VerifierVersion = *verifierVersion
	}
	if diagnosticCode != nil {
		checkpoint.DiagnosticCode = *diagnosticCode
	}
	checkpoint.CompletedAt = completedAt
	if err := checkpoint.Validate(); err != nil {
		return model.ExportCheckpoint{}, false, fmt.Errorf("validate stored PostgreSQL export checkpoint: %w", err)
	}
	return checkpoint, true, nil
}

func load(ctx context.Context, database queryer, documentID model.DocumentID, lock bool) (model.Snapshot, []model.AcceptedOperation, model.Revision, error) {
	query := `SELECT snapshot, snapshot_revision, current_revision FROM collaboration_documents WHERE document_id = $1`
	if lock {
		query += " FOR UPDATE"
	}
	var snapshotData []byte
	var snapshotRevision, currentRevision int64
	if err := database.QueryRow(ctx, query, documentID).Scan(&snapshotData, &snapshotRevision, &currentRevision); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Snapshot{}, nil, 0, collabstore.ErrSessionMissing
		}
		return model.Snapshot{}, nil, 0, fmt.Errorf("load PostgreSQL snapshot: %w", err)
	}
	var snapshot model.Snapshot
	if err := json.Unmarshal(snapshotData, &snapshot); err != nil {
		return model.Snapshot{}, nil, 0, fmt.Errorf("decode PostgreSQL snapshot: %w", err)
	}
	rows, err := database.Query(ctx, `SELECT accepted FROM collaboration_operations WHERE document_id = $1 AND revision > $2 ORDER BY revision`, documentID, snapshotRevision)
	if err != nil {
		return model.Snapshot{}, nil, 0, fmt.Errorf("query PostgreSQL replay: %w", err)
	}
	defer rows.Close()
	replay := make([]model.AcceptedOperation, 0)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return model.Snapshot{}, nil, 0, fmt.Errorf("scan PostgreSQL replay: %w", err)
		}
		accepted, err := decodeAccepted(data)
		if err != nil {
			return model.Snapshot{}, nil, 0, err
		}
		replay = append(replay, accepted)
	}
	if err := rows.Err(); err != nil {
		return model.Snapshot{}, nil, 0, fmt.Errorf("iterate PostgreSQL replay: %w", err)
	}
	return model.CloneSnapshot(snapshot), replay, model.Revision(currentRevision), nil
}

func lookupOperation(ctx context.Context, database queryer, documentID model.DocumentID, operationID model.OperationID) (model.AcceptedOperation, bool, error) {
	var data []byte
	err := database.QueryRow(ctx, `SELECT accepted FROM collaboration_operations WHERE document_id = $1 AND operation_id = $2`, documentID, operationID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.AcceptedOperation{}, false, nil
	}
	if err != nil {
		return model.AcceptedOperation{}, false, fmt.Errorf("lookup PostgreSQL operation: %w", err)
	}
	accepted, err := decodeAccepted(data)
	return accepted, true, err
}

func documentExists(ctx context.Context, database queryer, documentID model.DocumentID) (bool, error) {
	var present int
	err := database.QueryRow(ctx, `SELECT 1 FROM collaboration_documents WHERE document_id = $1`, documentID).Scan(&present)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func missingOrError(err error) error {
	if err != nil {
		return err
	}
	return collabstore.ErrSessionMissing
}

func replayDocument(snapshot model.Snapshot, operations []model.AcceptedOperation) (*engine.Document, error) {
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL stored snapshot: %w", err)
	}
	for _, accepted := range operations {
		verified, err := document.Apply(accepted.Operation, accepted.AcceptedAt)
		if err != nil {
			return nil, fmt.Errorf("replay PostgreSQL revision %d: %w", accepted.Revision, err)
		}
		if !reflect.DeepEqual(verified, accepted) {
			return nil, fmt.Errorf("replay PostgreSQL revision %d differs from stored operation", accepted.Revision)
		}
	}
	return document, nil
}

func replaySnapshotAt(snapshot model.Snapshot, operations []model.AcceptedOperation, revision model.Revision) (model.Snapshot, error) {
	if revision < snapshot.Revision {
		return model.Snapshot{}, fmt.Errorf("snapshot revision %d predates retained revision %d", revision, snapshot.Revision)
	}
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		return model.Snapshot{}, err
	}
	if revision == snapshot.Revision {
		return document.Snapshot(), nil
	}
	for _, accepted := range operations {
		verified, err := document.Apply(accepted.Operation, accepted.AcceptedAt)
		if err != nil || !reflect.DeepEqual(verified, accepted) {
			return model.Snapshot{}, fmt.Errorf("replay PostgreSQL revision %d failed: %w", accepted.Revision, err)
		}
		if accepted.Revision == revision {
			return document.Snapshot(), nil
		}
	}
	return model.Snapshot{}, fmt.Errorf("snapshot revision %d is not retained", revision)
}

func decodeAccepted(data []byte) (model.AcceptedOperation, error) {
	var accepted model.AcceptedOperation
	if err := json.Unmarshal(data, &accepted); err != nil {
		return model.AcceptedOperation{}, fmt.Errorf("decode PostgreSQL accepted operation: %w", err)
	}
	return model.CloneAcceptedOperation(accepted), nil
}
