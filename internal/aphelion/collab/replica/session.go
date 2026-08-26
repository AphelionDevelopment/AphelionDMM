package replica

import (
	"context"
	"fmt"
	"sync"
	"time"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/store"
)

type State string

const (
	StateCaughtUp       State = "caught_up"
	StateReadOnly       State = "read_only"
	StatePaused         State = "paused"
	StateDesynchronized State = "desynchronized"
)

type Session struct {
	mutex sync.RWMutex
	store store.ClientSessionStore
	local store.LocalSession
	state State
}

func (session *Session) Snapshot(ctx context.Context) (model.Snapshot, error) {
	session.mutex.RLock()
	documentID := session.local.DocumentID
	session.mutex.RUnlock()
	base, replay, err := session.store.Load(ctx, documentID)
	if err != nil {
		return model.Snapshot{}, err
	}
	document, err := engine.NewDocument(base)
	if err != nil {
		return model.Snapshot{}, err
	}
	for _, accepted := range replay {
		if _, err := document.Apply(accepted.Operation, accepted.AcceptedAt); err != nil {
			return model.Snapshot{}, err
		}
	}
	return document.Snapshot(), nil
}

func (session *Session) SavePending(ctx context.Context, operation model.Operation) error {
	session.mutex.RLock()
	sessionID := session.local.SessionID
	session.mutex.RUnlock()
	return session.store.SavePending(ctx, store.PendingSubmission{SessionID: sessionID, Operation: operation, Disposition: store.PendingReady, CreatedAt: time.Now().UTC()})
}

func (session *Session) ResolvePending(ctx context.Context, operationID model.OperationID, disposition store.PendingDisposition, diagnostic string) error {
	session.mutex.RLock()
	sessionID := session.local.SessionID
	session.mutex.RUnlock()
	return session.store.ResolvePending(ctx, sessionID, operationID, disposition, diagnostic)
}

func NewSession(value store.ClientSessionStore, local store.LocalSession) (*Session, error) {
	if value == nil {
		return nil, fmt.Errorf("replica store is unavailable")
	}
	if local.SessionID == "" || local.DocumentID == "" || local.OwnerPublicKey == (protocolv2.ActorKey{}) {
		return nil, fmt.Errorf("replica session is incomplete")
	}
	state := StateCaughtUp
	if local.Role == protocolv2.RoleViewer {
		state = StateReadOnly
	}
	return &Session{store: value, local: local, state: state}, nil
}

func (session *Session) State() State {
	session.mutex.RLock()
	defer session.mutex.RUnlock()
	return session.state
}

func (session *Session) LocalSession() store.LocalSession {
	session.mutex.RLock()
	defer session.mutex.RUnlock()
	local := session.local
	local.Manifest = append([]byte(nil), local.Manifest...)
	return local
}

func (session *Session) MarkDesynchronized() {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	session.state = StateDesynchronized
}

func (session *Session) SetOwnerOnline(online bool) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if !online {
		session.state = StatePaused
		return
	}
	if session.state == StatePaused {
		if session.local.Role == protocolv2.RoleViewer {
			session.state = StateReadOnly
		} else {
			session.state = StateCaughtUp
		}
	}
}

func (session *Session) ApplyAccepted(ctx context.Context, sender protocolv2.ActorKey, accepted protocolv2.OperationAccepted) (protocolv2.RevisionAcknowledged, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if sender != session.local.OwnerPublicKey {
		return protocolv2.RevisionAcknowledged{}, fmt.Errorf("accepted operation sender is not the owner")
	}
	if session.state == StateDesynchronized {
		return protocolv2.RevisionAcknowledged{}, fmt.Errorf("replica is desynchronized")
	}
	if accepted.Operation.DocumentID != session.local.DocumentID || accepted.Operation.Revision != session.local.AcknowledgedRevision+1 {
		session.state = StateDesynchronized
		return protocolv2.RevisionAcknowledged{}, fmt.Errorf("accepted operation creates a revision gap")
	}
	if err := session.store.ApplyAccepted(ctx, session.local.SessionID, accepted.Operation, accepted.MapHash); err != nil {
		return protocolv2.RevisionAcknowledged{}, err
	}
	session.local.AcknowledgedRevision = accepted.Operation.Revision
	session.local.AcknowledgedMapHash = accepted.MapHash
	return protocolv2.RevisionAcknowledged{Revision: accepted.Operation.Revision, MapHash: accepted.MapHash}, nil
}

func (session *Session) InstallReplay(ctx context.Context, sender protocolv2.ActorKey, replay protocolv2.SyncReplay) (protocolv2.SyncComplete, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if sender != session.local.OwnerPublicKey {
		return protocolv2.SyncComplete{}, fmt.Errorf("sync replay sender is not the owner")
	}
	base, retained, err := session.store.Load(ctx, session.local.DocumentID)
	if err != nil {
		return protocolv2.SyncComplete{}, err
	}
	combined := append([]model.AcceptedOperation(nil), retained...)
	for _, accepted := range replay.Operations {
		if accepted.Revision > session.local.AcknowledgedRevision {
			combined = append(combined, model.CloneAcceptedOperation(accepted))
		}
	}
	updated := session.local
	updated.AcknowledgedRevision = replay.Revision
	updated.AcknowledgedMapHash = replay.MapHash
	if err := session.store.ReplaceReplica(ctx, updated, base, combined); err != nil {
		session.state = StateDesynchronized
		return protocolv2.SyncComplete{}, err
	}
	session.local = updated
	session.state = session.readyState()
	return protocolv2.SyncComplete{Revision: replay.Revision, MapHash: replay.MapHash}, nil
}

func (session *Session) InstallSnapshot(ctx context.Context, sender protocolv2.ActorKey, incoming protocolv2.SyncSnapshot) (protocolv2.SyncComplete, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if sender != session.local.OwnerPublicKey {
		return protocolv2.SyncComplete{}, fmt.Errorf("sync snapshot sender is not the owner")
	}
	if incoming.Snapshot.DocumentID != session.local.DocumentID {
		session.state = StateDesynchronized
		return protocolv2.SyncComplete{}, fmt.Errorf("sync snapshot document does not match replica")
	}
	hash, err := incoming.Snapshot.Hash()
	if err != nil || hash != incoming.MapHash {
		session.state = StateDesynchronized
		return protocolv2.SyncComplete{}, fmt.Errorf("sync snapshot hash is invalid")
	}
	updated := session.local
	updated.AcknowledgedRevision = incoming.Snapshot.Revision
	updated.AcknowledgedMapHash = incoming.MapHash
	if err := session.store.ReplaceReplica(ctx, updated, incoming.Snapshot, nil); err != nil {
		session.state = StateDesynchronized
		return protocolv2.SyncComplete{}, err
	}
	session.local = updated
	session.state = session.readyState()
	return protocolv2.SyncComplete{Revision: incoming.Snapshot.Revision, MapHash: incoming.MapHash}, nil
}

func (session *Session) readyState() State {
	if session.local.Role == protocolv2.RoleViewer {
		return StateReadOnly
	}
	return StateCaughtUp
}
