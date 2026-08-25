package store

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
)

type MemoryStore struct {
	mutex      sync.RWMutex
	closed     bool
	sessions   map[model.DocumentID]model.Snapshot
	operations map[model.DocumentID][]model.AcceptedOperation
	accepted   map[model.DocumentID]map[model.OperationID]model.AcceptedOperation
	documents  map[model.DocumentID]*engine.Document
	hashes     map[model.DocumentID]map[model.Revision]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions:   make(map[model.DocumentID]model.Snapshot),
		operations: make(map[model.DocumentID][]model.AcceptedOperation),
		accepted:   make(map[model.DocumentID]map[model.OperationID]model.AcceptedOperation),
		documents:  make(map[model.DocumentID]*engine.Document),
		hashes:     make(map[model.DocumentID]map[model.Revision]string),
	}
}

func (store *MemoryStore) Create(ctx context.Context, snapshot model.Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.closed {
		return ErrStoreClosed
	}
	if _, exists := store.sessions[snapshot.DocumentID]; exists {
		return ErrSessionExists
	}
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		return err
	}
	mapHash, err := snapshot.Hash()
	if err != nil {
		return err
	}
	store.sessions[snapshot.DocumentID] = model.CloneSnapshot(snapshot)
	store.operations[snapshot.DocumentID] = nil
	store.accepted[snapshot.DocumentID] = make(map[model.OperationID]model.AcceptedOperation)
	store.documents[snapshot.DocumentID] = document
	store.hashes[snapshot.DocumentID] = map[model.Revision]string{snapshot.Revision: mapHash}
	return nil
}

func (store *MemoryStore) Append(ctx context.Context, accepted model.AcceptedOperation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.closed {
		return ErrStoreClosed
	}
	if _, exists := store.sessions[accepted.DocumentID]; !exists {
		return ErrSessionMissing
	}
	if prior, exists := store.accepted[accepted.DocumentID][accepted.OperationID]; exists {
		if !reflect.DeepEqual(prior, accepted) {
			return fmt.Errorf("operation %q conflicts with stored revision %d", accepted.OperationID, prior.Revision)
		}
		return nil
	}
	wantRevision := store.sessions[accepted.DocumentID].Revision + model.Revision(len(store.operations[accepted.DocumentID])) + 1
	if accepted.Revision != wantRevision {
		return fmt.Errorf("operation revision is %d, want %d", accepted.Revision, wantRevision)
	}
	candidate := store.documents[accepted.DocumentID].Clone()
	verified, err := candidate.Apply(accepted.Operation, accepted.AcceptedAt)
	if err != nil {
		return fmt.Errorf("verify accepted operation: %w", err)
	}
	if !reflect.DeepEqual(verified, accepted) {
		return fmt.Errorf("accepted operation does not match verified operation")
	}
	mapHash, err := candidate.Snapshot().Hash()
	if err != nil {
		return fmt.Errorf("hash accepted revision: %w", err)
	}
	cloned := model.CloneAcceptedOperation(accepted)
	store.operations[accepted.DocumentID] = append(store.operations[accepted.DocumentID], cloned)
	store.accepted[accepted.DocumentID][accepted.OperationID] = cloned
	store.documents[accepted.DocumentID] = candidate
	store.hashes[accepted.DocumentID][accepted.Revision] = mapHash
	return nil
}

func (store *MemoryStore) Load(ctx context.Context, documentID model.DocumentID) (model.Snapshot, []model.AcceptedOperation, error) {
	if err := ctx.Err(); err != nil {
		return model.Snapshot{}, nil, err
	}
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return model.Snapshot{}, nil, ErrStoreClosed
	}
	snapshot, exists := store.sessions[documentID]
	if !exists {
		return model.Snapshot{}, nil, ErrSessionMissing
	}
	operations := make([]model.AcceptedOperation, len(store.operations[documentID]))
	for index, accepted := range store.operations[documentID] {
		operations[index] = model.CloneAcceptedOperation(accepted)
	}
	return model.CloneSnapshot(snapshot), operations, nil
}

func (store *MemoryStore) SaveSnapshot(ctx context.Context, snapshot model.Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.closed {
		return ErrStoreClosed
	}
	if _, exists := store.sessions[snapshot.DocumentID]; !exists {
		return ErrSessionMissing
	}
	if _, err := engine.NewDocument(snapshot); err != nil {
		return err
	}
	mapHash, err := snapshot.Hash()
	if err != nil {
		return err
	}
	retainedHash, exists := store.hashes[snapshot.DocumentID][snapshot.Revision]
	if !exists || retainedHash != mapHash {
		return fmt.Errorf("snapshot revision %d is not retained with the supplied hash", snapshot.Revision)
	}
	base := store.sessions[snapshot.DocumentID]
	current := store.documents[snapshot.DocumentID].Snapshot()
	if snapshot.Revision < base.Revision || snapshot.Revision > current.Revision {
		return fmt.Errorf("snapshot revision %d is outside retained range %d through %d", snapshot.Revision, base.Revision, current.Revision)
	}
	store.sessions[snapshot.DocumentID] = model.CloneSnapshot(snapshot)
	retained := store.operations[snapshot.DocumentID][:0]
	for _, accepted := range store.operations[snapshot.DocumentID] {
		if accepted.Revision > snapshot.Revision {
			retained = append(retained, accepted)
		}
	}
	store.operations[snapshot.DocumentID] = retained
	return nil
}

func (store *MemoryStore) RevisionHash(ctx context.Context, documentID model.DocumentID, revision model.Revision) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return "", false, ErrStoreClosed
	}
	hashes, exists := store.hashes[documentID]
	if !exists {
		return "", false, ErrSessionMissing
	}
	hash, exists := hashes[revision]
	return hash, exists, nil
}

func (store *MemoryStore) LookupOperation(ctx context.Context, documentID model.DocumentID, operationID model.OperationID) (model.AcceptedOperation, bool, error) {
	if err := ctx.Err(); err != nil {
		return model.AcceptedOperation{}, false, err
	}
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return model.AcceptedOperation{}, false, ErrStoreClosed
	}
	operations, exists := store.accepted[documentID]
	if !exists {
		return model.AcceptedOperation{}, false, ErrSessionMissing
	}
	accepted, exists := operations[operationID]
	return model.CloneAcceptedOperation(accepted), exists, nil
}

func (store *MemoryStore) Close() error {
	store.mutex.Lock()
	store.closed = true
	store.mutex.Unlock()
	return nil
}
