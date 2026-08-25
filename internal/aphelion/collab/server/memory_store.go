package server

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
)

var (
	ErrSessionExists  = errors.New("session already exists")
	ErrSessionMissing = errors.New("session does not exist")
)

type SessionStore interface {
	Create(context.Context, model.Snapshot) error
	Append(context.Context, model.AcceptedOperation) error
	Load(context.Context, model.DocumentID) (model.Snapshot, []model.AcceptedOperation, error)
	SaveSnapshot(context.Context, model.Snapshot) error
	LookupOperation(context.Context, model.DocumentID, model.OperationID) (model.AcceptedOperation, bool, error)
}

type MemoryStore struct {
	mutex      sync.RWMutex
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
	if _, exists := store.sessions[accepted.DocumentID]; !exists {
		return ErrSessionMissing
	}
	if prior, exists := store.accepted[accepted.DocumentID][accepted.OperationID]; exists {
		if prior.Revision != accepted.Revision {
			return fmt.Errorf("operation %q already stored at revision %d", accepted.OperationID, prior.Revision)
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
	if verified.Revision != accepted.Revision {
		return fmt.Errorf("verified revision is %d, want %d", verified.Revision, accepted.Revision)
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
	if _, exists := store.sessions[snapshot.DocumentID]; !exists {
		return ErrSessionMissing
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
	store.documents[snapshot.DocumentID] = document
	store.hashes[snapshot.DocumentID][snapshot.Revision] = mapHash
	return nil
}

func (store *MemoryStore) RevisionHash(ctx context.Context, documentID model.DocumentID, revision model.Revision) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	store.mutex.RLock()
	defer store.mutex.RUnlock()
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
	operations, exists := store.accepted[documentID]
	if !exists {
		return model.AcceptedOperation{}, false, ErrSessionMissing
	}
	accepted, exists := operations[operationID]
	return model.CloneAcceptedOperation(accepted), exists, nil
}
