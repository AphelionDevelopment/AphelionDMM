package server

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	collabtelemetry "sdmm/internal/aphelion/collab/telemetry"
)

type RecoveryError struct {
	DocumentID model.DocumentID
	Err        error
}

func (failure *RecoveryError) Error() string {
	return fmt.Sprintf("recover document %q: %v", failure.DocumentID, failure.Err)
}

func (failure *RecoveryError) Unwrap() error {
	return failure.Err
}

func startOrRecoverDocument(ctx context.Context, snapshot model.Snapshot, store SessionStore, config DocumentConfig) (*DocumentOwner, error) {
	owner, err := StartDocumentWithConfig(ctx, snapshot, store, config)
	if err == nil {
		return owner, nil
	}
	if !errors.Is(err, ErrSessionExists) {
		return nil, err
	}
	suppliedHash, hashErr := snapshot.Hash()
	if hashErr != nil {
		return nil, hashErr
	}
	retainedHash, found, hashErr := store.RevisionHash(ctx, snapshot.DocumentID, snapshot.Revision)
	if hashErr != nil {
		return nil, &RecoveryError{DocumentID: snapshot.DocumentID, Err: fmt.Errorf("verify recovery baseline: %w", hashErr)}
	}
	if !found || retainedHash != suppliedHash {
		return nil, &RecoveryError{DocumentID: snapshot.DocumentID, Err: fmt.Errorf("recovery baseline does not match retained revision %d", snapshot.Revision)}
	}
	owner, err = RecoverDocumentWithConfig(ctx, snapshot.DocumentID, store, config)
	if err != nil {
		return nil, &RecoveryError{DocumentID: snapshot.DocumentID, Err: err}
	}
	return owner, nil
}

func RecoverDocument(ctx context.Context, documentID model.DocumentID, store SessionStore) (*DocumentOwner, error) {
	return RecoverDocumentWithConfig(ctx, documentID, store, DocumentConfig{})
}

func RecoverDocumentWithConfig(ctx context.Context, documentID model.DocumentID, store SessionStore, config DocumentConfig) (*DocumentOwner, error) {
	if store == nil {
		return nil, fmt.Errorf("recover document: store is nil")
	}
	loadContext := ctx
	finishLoad := func(error) {}
	if config.Telemetry != nil {
		loadContext, finishLoad = config.Telemetry.Store(ctx, collabtelemetry.StoreLoad)
	}
	snapshot, replay, err := store.Load(loadContext, documentID)
	if err != nil {
		finishLoad(err)
		return nil, fmt.Errorf("load stored document: %w", err)
	}
	finishLoad(nil)
	finishReplay := func(error) {}
	if config.Telemetry != nil {
		_, finishReplay = config.Telemetry.Replay(ctx, len(replay))
	}
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		finishReplay(err)
		return nil, fmt.Errorf("open stored snapshot: %w", err)
	}
	for _, accepted := range replay {
		verified, err := document.Apply(accepted.Operation, accepted.AcceptedAt)
		if err != nil {
			finishReplay(err)
			return nil, fmt.Errorf("replay revision %d: %w", accepted.Revision, err)
		}
		if !reflect.DeepEqual(verified, accepted) {
			err := fmt.Errorf("replay revision %d differs from stored operation", accepted.Revision)
			finishReplay(err)
			return nil, err
		}
	}
	finishReplay(nil)
	return startDocument(ctx, document, store, config), nil
}
