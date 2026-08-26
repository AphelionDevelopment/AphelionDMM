package store

import (
	"context"
	"time"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
)

type PendingDisposition string

const (
	PendingReady       PendingDisposition = "ready"
	PendingConflicting PendingDisposition = "conflicting"
	PendingObsolete    PendingDisposition = "obsolete"
	PendingSubmitted   PendingDisposition = "submitted"
)

type LocalSession struct {
	SessionID            string
	RelayURL             string
	Role                 protocolv2.Role
	DocumentID           model.DocumentID
	OwnerPublicKey       protocolv2.ActorKey
	Manifest             []byte
	ManifestSHA256       [32]byte
	AcknowledgedRevision model.Revision
	AcknowledgedMapHash  string
	DisplayName          string
	IdentitySecretRef    string
	GroupKeySecretRef    string
	OwnerSecretRef       string
}

type PendingSubmission struct {
	SessionID   string
	Operation   model.Operation
	Disposition PendingDisposition
	CreatedAt   time.Time
	Diagnostic  string
}

type Admission struct {
	SessionID    string
	CapabilityID [32]byte
	Role         protocolv2.Role
	ExpiresAt    time.Time
	BoundActor   protocolv2.ActorKey
}

type ClientSessionStore interface {
	SessionStore
	CreateClientSession(context.Context, LocalSession) error
	LoadClientSession(context.Context, string) (LocalSession, error)
	SavePending(context.Context, PendingSubmission) error
	ListPending(context.Context, string) ([]PendingSubmission, error)
	ResolvePending(context.Context, string, model.OperationID, PendingDisposition, string) error
	ApplyAccepted(context.Context, string, model.AcceptedOperation, string) error
	ReplaceReplica(context.Context, LocalSession, model.Snapshot, []model.AcceptedOperation) error
	SaveManifest(context.Context, string, []byte, []Admission) error
	ListAdmissions(context.Context, string) ([]Admission, error)
}
