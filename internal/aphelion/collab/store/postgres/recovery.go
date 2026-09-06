package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	collabstore "sdmm/internal/aphelion/collab/store"
)

func (store *Store) LoadRecovery(ctx context.Context, documentID model.DocumentID) (engine.RecoveryState, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return engine.RecoveryState{}, collabstore.ErrStoreClosed
	}
	transaction, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return engine.RecoveryState{}, err
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	state, err := loadRecovery(ctx, transaction, documentID, false)
	if err != nil {
		return engine.RecoveryState{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return engine.RecoveryState{}, err
	}
	return state, nil
}

func loadRecovery(ctx context.Context, database queryer, documentID model.DocumentID, lock bool) (engine.RecoveryState, error) {
	var state engine.RecoveryState
	var data []byte
	var revision model.Revision
	query := "SELECT snapshot, snapshot_revision, snapshot_hash, current_revision, current_hash FROM collaboration_documents WHERE document_id = $1"
	if lock {
		query += " FOR UPDATE"
	}
	err := database.QueryRow(ctx, query, documentID).Scan(&data, &revision, &state.SnapshotHash, &state.HeadRevision, &state.HeadHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, collabstore.ErrSessionMissing
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state.Snapshot); err != nil {
		return state, fmt.Errorf("decode snapshot: %w", err)
	}
	if state.Snapshot.DocumentID != documentID || state.Snapshot.Revision != revision {
		return state, fmt.Errorf("stored snapshot identity/revision differs from row")
	}
	state.Hashes = make(map[model.Revision]string)
	rows, err := database.Query(ctx, "SELECT revision, map_hash FROM collaboration_revision_hashes WHERE document_id = $1 ORDER BY revision", documentID)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var hash string
		if err := rows.Scan(&revision, &hash); err != nil {
			rows.Close()
			return state, err
		}
		state.Hashes[revision] = hash
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	rows, err = database.Query(ctx, "SELECT operation_id, revision, accepted, map_hash FROM collaboration_operations WHERE document_id = $1 ORDER BY revision", documentID)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	for rows.Next() {
		var operationID model.OperationID
		var hash string
		if err := rows.Scan(&operationID, &revision, &data, &hash); err != nil {
			return state, err
		}
		accepted, err := decodeAccepted(data)
		if err != nil {
			return state, err
		}
		if accepted.DocumentID != documentID || accepted.OperationID != operationID || accepted.Revision != revision || hash != state.Hashes[revision] {
			return state, fmt.Errorf("stored operation identity/revision/hash differs from row/ledger")
		}
		state.Operations = append(state.Operations, accepted)
	}
	if err := rows.Err(); err != nil {
		return state, err
	}
	return state, nil
}
