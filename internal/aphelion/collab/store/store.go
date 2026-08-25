package store

import (
	"context"
	"errors"

	"sdmm/internal/aphelion/collab/model"
)

var (
	ErrSessionExists  = errors.New("session already exists")
	ErrSessionMissing = errors.New("session does not exist")
	ErrStoreClosed    = errors.New("session store is closed")
)

type SessionStore interface {
	Create(context.Context, model.Snapshot) error
	Append(context.Context, model.AcceptedOperation) error
	Load(context.Context, model.DocumentID) (model.Snapshot, []model.AcceptedOperation, error)
	SaveSnapshot(context.Context, model.Snapshot) error
	RevisionHash(context.Context, model.DocumentID, model.Revision) (string, bool, error)
	LookupOperation(context.Context, model.DocumentID, model.OperationID) (model.AcceptedOperation, bool, error)
	Close() error
}
