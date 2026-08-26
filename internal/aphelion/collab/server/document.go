package server

import (
	"context"

	"sdmm/internal/aphelion/collab/authority"
	"sdmm/internal/aphelion/collab/model"
)

var ErrDocumentClosed = authority.ErrDocumentClosed

type DocumentOwner struct {
	document *authority.Document
}

func StartDocument(ctx context.Context, snapshot model.Snapshot, store SessionStore) (*DocumentOwner, error) {
	return StartDocumentWithConfig(ctx, snapshot, store, DocumentConfig{})
}

func StartDocumentWithConfig(ctx context.Context, snapshot model.Snapshot, store SessionStore, config DocumentConfig) (*DocumentOwner, error) {
	document, err := authority.StartDocumentWithConfig(ctx, snapshot, store, config)
	if err != nil {
		return nil, err
	}
	return &DocumentOwner{document: document}, nil
}

func (owner *DocumentOwner) Submit(ctx context.Context, operation model.Operation) (model.AcceptedOperation, error) {
	accepted, _, err := owner.document.Submit(ctx, operation)
	return accepted, err
}

func (owner *DocumentOwner) SubmitWithStatus(ctx context.Context, operation model.Operation) (model.AcceptedOperation, bool, error) {
	return owner.document.Submit(ctx, operation)
}

func (owner *DocumentOwner) Snapshot(ctx context.Context) (model.Snapshot, error) {
	return owner.document.Snapshot(ctx)
}

func (owner *DocumentOwner) BuildInverse(ctx context.Context, actorID model.ActorID, targetID, inverseID model.OperationID) (model.Operation, error) {
	return owner.document.BuildInverse(ctx, actorID, targetID, inverseID)
}

func (owner *DocumentOwner) Close(ctx context.Context) error {
	return owner.document.Close(ctx)
}
