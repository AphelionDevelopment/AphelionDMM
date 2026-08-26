package server

import (
	"context"
	"errors"
	"fmt"

	"sdmm/internal/aphelion/collab/authority"
	"sdmm/internal/aphelion/collab/model"
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
	document, err := authority.RecoverDocumentWithConfig(ctx, documentID, store, config)
	if err != nil {
		return nil, err
	}
	return &DocumentOwner{document: document}, nil
}
