package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	collabstore "sdmm/internal/aphelion/collab/store"

	_ "modernc.org/sqlite"
)

const minimumSQLiteVersion = "3.51.3"

type Store struct {
	mutex    sync.RWMutex
	database *sql.DB
	version  string
	closed   bool
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open SQLite store: path is empty")
	}
	parameters := url.Values{}
	parameters.Add("_pragma", "busy_timeout(5000)")
	parameters.Add("_pragma", "foreign_keys(1)")
	parameters.Add("_pragma", "journal_mode(WAL)")
	parameters.Add("_pragma", "synchronous(FULL)")
	dsn := "file:" + filepath.ToSlash(path) + "?" + parameters.Encode()
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	ctx := context.Background()
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping SQLite database: %w", err)
	}
	var version string
	if err := database.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("query SQLite version: %w", err)
	}
	if compareVersions(version, minimumSQLiteVersion) < 0 {
		_ = database.Close()
		return nil, fmt.Errorf("SQLite version is %s, require at least %s", version, minimumSQLiteVersion)
	}
	if err := verifyPragmas(ctx, database); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := migrate(ctx, database); err != nil {
		_ = database.Close()
		return nil, err
	}
	return &Store{database: database, version: version}, nil
}

func (store *Store) Version() string {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	return store.version
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
	if _, err := engine.NewDocument(snapshot); err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	mapHash, err := snapshot.Hash()
	if err != nil {
		return err
	}
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	var present int
	err = transaction.QueryRowContext(ctx, "SELECT 1 FROM documents WHERE document_id = ?", snapshot.DocumentID).Scan(&present)
	if err == nil {
		return collabstore.ErrSessionExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check existing document: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, "INSERT INTO documents(document_id, snapshot, snapshot_revision, snapshot_hash) VALUES(?, ?, ?, ?)", snapshot.DocumentID, encoded, snapshot.Revision, mapHash); err != nil {
		return fmt.Errorf("insert document: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, "INSERT INTO revision_hashes(document_id, revision, map_hash) VALUES(?, ?, ?)", snapshot.DocumentID, snapshot.Revision, mapHash); err != nil {
		return fmt.Errorf("insert initial revision hash: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit create: %w", err)
	}
	return nil
}

func (store *Store) Append(ctx context.Context, accepted model.AcceptedOperation) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin append: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	var priorData []byte
	err = transaction.QueryRowContext(ctx, "SELECT accepted FROM operations WHERE document_id = ? AND operation_id = ?", accepted.DocumentID, accepted.OperationID).Scan(&priorData)
	if err == nil {
		prior, decodeErr := decodeAccepted(priorData)
		if decodeErr != nil {
			return decodeErr
		}
		if !reflect.DeepEqual(prior, accepted) {
			return fmt.Errorf("operation %q conflicts with stored revision %d", accepted.OperationID, prior.Revision)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lookup duplicate operation: %w", err)
	}
	snapshot, replay, err := load(ctx, transaction, accepted.DocumentID)
	if err != nil {
		return err
	}
	document, err := replayDocument(snapshot, replay)
	if err != nil {
		return err
	}
	verified, err := document.Apply(accepted.Operation, accepted.AcceptedAt)
	if err != nil {
		return fmt.Errorf("verify accepted operation: %w", err)
	}
	if !reflect.DeepEqual(verified, accepted) {
		return fmt.Errorf("accepted operation does not match verified operation")
	}
	mapHash, err := document.Snapshot().Hash()
	if err != nil {
		return fmt.Errorf("hash accepted revision: %w", err)
	}
	encoded, err := json.Marshal(accepted)
	if err != nil {
		return fmt.Errorf("encode accepted operation: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, "INSERT INTO operations(document_id, operation_id, revision, accepted, map_hash) VALUES(?, ?, ?, ?, ?)", accepted.DocumentID, accepted.OperationID, accepted.Revision, encoded, mapHash); err != nil {
		return fmt.Errorf("insert operation: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, "INSERT INTO revision_hashes(document_id, revision, map_hash) VALUES(?, ?, ?)", accepted.DocumentID, accepted.Revision, mapHash); err != nil {
		return fmt.Errorf("insert revision hash: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit append: %w", err)
	}
	return nil
}

func (store *Store) Load(ctx context.Context, documentID model.DocumentID) (model.Snapshot, []model.AcceptedOperation, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return model.Snapshot{}, nil, collabstore.ErrStoreClosed
	}
	if err := ctx.Err(); err != nil {
		return model.Snapshot{}, nil, err
	}
	return load(ctx, store.database, documentID)
}

func (store *Store) SaveSnapshot(ctx context.Context, snapshot model.Snapshot) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin snapshot: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	base, replay, err := load(ctx, transaction, snapshot.DocumentID)
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
		return fmt.Errorf("encode snapshot: %w", err)
	}
	result, err := transaction.ExecContext(ctx, "UPDATE documents SET snapshot = ?, snapshot_revision = ?, snapshot_hash = ? WHERE document_id = ?", encoded, snapshot.Revision, mapHash, snapshot.DocumentID)
	if err != nil {
		return fmt.Errorf("update snapshot: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read snapshot update result: %w", err)
	}
	if rows != 1 {
		return collabstore.ErrSessionMissing
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit snapshot: %w", err)
	}
	return nil
}

func (store *Store) RevisionHash(ctx context.Context, documentID model.DocumentID, revision model.Revision) (string, bool, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return "", false, collabstore.ErrStoreClosed
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	var hash string
	err := store.database.QueryRowContext(ctx, "SELECT map_hash FROM revision_hashes WHERE document_id = ? AND revision = ?", documentID, revision).Scan(&hash)
	if err == nil {
		return hash, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("query revision hash: %w", err)
	}
	var present int
	err = store.database.QueryRowContext(ctx, "SELECT 1 FROM documents WHERE document_id = ?", documentID).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, collabstore.ErrSessionMissing
	}
	if err != nil {
		return "", false, fmt.Errorf("query document: %w", err)
	}
	return "", false, nil
}

func (store *Store) LookupOperation(ctx context.Context, documentID model.DocumentID, operationID model.OperationID) (model.AcceptedOperation, bool, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return model.AcceptedOperation{}, false, collabstore.ErrStoreClosed
	}
	if err := ctx.Err(); err != nil {
		return model.AcceptedOperation{}, false, err
	}
	var data []byte
	err := store.database.QueryRowContext(ctx, "SELECT accepted FROM operations WHERE document_id = ? AND operation_id = ?", documentID, operationID).Scan(&data)
	if err == nil {
		accepted, decodeErr := decodeAccepted(data)
		return accepted, true, decodeErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return model.AcceptedOperation{}, false, fmt.Errorf("lookup operation: %w", err)
	}
	var present int
	err = store.database.QueryRowContext(ctx, "SELECT 1 FROM documents WHERE document_id = ?", documentID).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return model.AcceptedOperation{}, false, collabstore.ErrSessionMissing
	}
	if err != nil {
		return model.AcceptedOperation{}, false, fmt.Errorf("query document: %w", err)
	}
	return model.AcceptedOperation{}, false, nil
}

func (store *Store) Close() error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.closed {
		return nil
	}
	store.closed = true
	return store.database.Close()
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func load(ctx context.Context, database queryer, documentID model.DocumentID) (model.Snapshot, []model.AcceptedOperation, error) {
	var snapshotData []byte
	var snapshotRevision model.Revision
	err := database.QueryRowContext(ctx, "SELECT snapshot, snapshot_revision FROM documents WHERE document_id = ?", documentID).Scan(&snapshotData, &snapshotRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Snapshot{}, nil, collabstore.ErrSessionMissing
	}
	if err != nil {
		return model.Snapshot{}, nil, fmt.Errorf("load snapshot: %w", err)
	}
	var snapshot model.Snapshot
	if err := json.Unmarshal(snapshotData, &snapshot); err != nil {
		return model.Snapshot{}, nil, fmt.Errorf("decode snapshot: %w", err)
	}
	rows, err := database.QueryContext(ctx, "SELECT accepted FROM operations WHERE document_id = ? AND revision > ? ORDER BY revision", documentID, snapshotRevision)
	if err != nil {
		return model.Snapshot{}, nil, fmt.Errorf("query replay: %w", err)
	}
	defer func() { _ = rows.Close() }()
	operations := make([]model.AcceptedOperation, 0)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return model.Snapshot{}, nil, fmt.Errorf("scan replay: %w", err)
		}
		accepted, err := decodeAccepted(data)
		if err != nil {
			return model.Snapshot{}, nil, err
		}
		operations = append(operations, accepted)
	}
	if err := rows.Err(); err != nil {
		return model.Snapshot{}, nil, fmt.Errorf("iterate replay: %w", err)
	}
	return model.CloneSnapshot(snapshot), operations, nil
}

func replayDocument(snapshot model.Snapshot, operations []model.AcceptedOperation) (*engine.Document, error) {
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		return nil, fmt.Errorf("open stored snapshot: %w", err)
	}
	for _, accepted := range operations {
		verified, err := document.Apply(accepted.Operation, accepted.AcceptedAt)
		if err != nil {
			return nil, fmt.Errorf("replay revision %d: %w", accepted.Revision, err)
		}
		if !reflect.DeepEqual(verified, accepted) {
			return nil, fmt.Errorf("replay revision %d differs from stored operation", accepted.Revision)
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
		return model.Snapshot{}, fmt.Errorf("open stored snapshot: %w", err)
	}
	if revision == snapshot.Revision {
		return document.Snapshot(), nil
	}
	for _, accepted := range operations {
		verified, err := document.Apply(accepted.Operation, accepted.AcceptedAt)
		if err != nil {
			return model.Snapshot{}, fmt.Errorf("replay revision %d: %w", accepted.Revision, err)
		}
		if !reflect.DeepEqual(verified, accepted) {
			return model.Snapshot{}, fmt.Errorf("replay revision %d differs from stored operation", accepted.Revision)
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
		return model.AcceptedOperation{}, fmt.Errorf("decode accepted operation: %w", err)
	}
	return model.CloneAcceptedOperation(accepted), nil
}

func verifyPragmas(ctx context.Context, database *sql.DB) error {
	expectations := []struct {
		name string
		want string
	}{
		{name: "foreign_keys", want: "1"},
		{name: "journal_mode", want: "wal"},
		{name: "busy_timeout", want: "5000"},
		{name: "synchronous", want: "2"},
	}
	for _, expectation := range expectations {
		var actual string
		if err := database.QueryRowContext(ctx, "PRAGMA "+expectation.name).Scan(&actual); err != nil {
			return fmt.Errorf("query PRAGMA %s: %w", expectation.name, err)
		}
		if !strings.EqualFold(actual, expectation.want) {
			return fmt.Errorf("PRAGMA %s is %q, want %q", expectation.name, actual, expectation.want)
		}
	}
	return nil
}

func compareVersions(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for index := 0; index < len(leftParts) || index < len(rightParts); index++ {
		var leftValue, rightValue int
		if index < len(leftParts) {
			leftValue, _ = strconv.Atoi(leftParts[index])
		}
		if index < len(rightParts) {
			rightValue, _ = strconv.Atoi(rightParts[index])
		}
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}
	return 0
}
