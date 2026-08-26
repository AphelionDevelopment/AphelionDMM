package authority

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/store"
	collabtelemetry "sdmm/internal/aphelion/collab/telemetry"
)

const documentRequestQueueSize = 256

var ErrDocumentClosed = errors.New("document authority is closed")

type Config struct {
	SnapshotOperationThreshold int
	SnapshotInterval           time.Duration
	OnSnapshotError            func(error)
	Telemetry                  *collabtelemetry.Telemetry
}

type Document struct {
	requests  chan request
	done      chan struct{}
	cancel    context.CancelFunc
	closeOnce sync.Once
	snapshots sync.WaitGroup
}

type requestKind uint8

const (
	requestSubmit requestKind = iota + 1
	requestSnapshot
	requestBuildInverse
)

type request struct {
	kind      requestKind
	operation model.Operation
	actorID   model.ActorID
	targetID  model.OperationID
	inverseID model.OperationID
	response  chan response
}

type response struct {
	accepted  model.AcceptedOperation
	snapshot  model.Snapshot
	duplicate bool
	err       error
}

func StartDocument(ctx context.Context, snapshot model.Snapshot, value store.SessionStore) (*Document, error) {
	return StartDocumentWithConfig(ctx, snapshot, value, Config{})
}

func StartDocumentWithConfig(ctx context.Context, snapshot model.Snapshot, value store.SessionStore, config Config) (*Document, error) {
	if value == nil {
		return nil, fmt.Errorf("start document: store is nil")
	}
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		return nil, fmt.Errorf("start document: %w", err)
	}
	storeContext, finishStore := observedStore(ctx, config.Telemetry, collabtelemetry.StoreCreate)
	if err := value.Create(storeContext, snapshot); err != nil {
		finishStore(err)
		return nil, fmt.Errorf("create stored session: %w", err)
	}
	finishStore(nil)
	return start(ctx, document, value, config), nil
}

func RecoverDocumentWithConfig(ctx context.Context, documentID model.DocumentID, value store.SessionStore, config Config) (*Document, error) {
	if value == nil {
		return nil, fmt.Errorf("recover document: store is nil")
	}
	loadContext, finishLoad := observedStore(ctx, config.Telemetry, collabtelemetry.StoreLoad)
	snapshot, replay, err := value.Load(loadContext, documentID)
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
	return start(ctx, document, value, config), nil
}

func (document *Document) Submit(ctx context.Context, operation model.Operation) (model.AcceptedOperation, bool, error) {
	result, err := document.request(ctx, request{kind: requestSubmit, operation: model.CloneOperation(operation)})
	return result.accepted, result.duplicate, err
}

func (document *Document) Snapshot(ctx context.Context) (model.Snapshot, error) {
	result, err := document.request(ctx, request{kind: requestSnapshot})
	return result.snapshot, err
}

func (document *Document) BuildInverse(ctx context.Context, actorID model.ActorID, targetID, inverseID model.OperationID) (model.Operation, error) {
	result, err := document.request(ctx, request{kind: requestBuildInverse, actorID: actorID, targetID: targetID, inverseID: inverseID})
	return result.accepted.Operation, err
}

func (document *Document) Close(ctx context.Context) error {
	document.closeOnce.Do(document.cancel)
	select {
	case <-document.done:
		document.snapshots.Wait()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func start(ctx context.Context, engineDocument *engine.Document, value store.SessionStore, config Config) *Document {
	runContext, cancel := context.WithCancel(ctx)
	document := &Document{requests: make(chan request, documentRequestQueueSize), done: make(chan struct{}), cancel: cancel}
	go document.run(runContext, engineDocument, value, config)
	return document
}

func (document *Document) request(ctx context.Context, value request) (response, error) {
	value.response = make(chan response, 1)
	select {
	case document.requests <- value:
	case <-document.done:
		return response{}, ErrDocumentClosed
	case <-ctx.Done():
		return response{}, ctx.Err()
	}
	select {
	case result := <-value.response:
		return result, result.err
	case <-document.done:
		return response{}, ErrDocumentClosed
	case <-ctx.Done():
		return response{}, ctx.Err()
	}
}

func (document *Document) run(ctx context.Context, engineDocument *engine.Document, value store.SessionStore, config Config) {
	defer close(document.done)
	acceptedSinceSnapshot := 0
	snapshotInFlight := false
	snapshotResults := make(chan snapshotResult, 1)
	var interval <-chan time.Time
	var ticker *time.Ticker
	if config.SnapshotInterval > 0 {
		ticker = time.NewTicker(config.SnapshotInterval)
		defer ticker.Stop()
		interval = ticker.C
	}
	scheduleSnapshot := func() {
		snapshotInFlight = true
		document.saveSnapshot(ctx, value, engineDocument.Snapshot(), snapshotResults, config.Telemetry)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-interval:
			if acceptedSinceSnapshot > 0 && !snapshotInFlight {
				scheduleSnapshot()
			}
		case result := <-snapshotResults:
			snapshotInFlight = false
			if result.err != nil {
				if config.OnSnapshotError != nil {
					config.OnSnapshotError(result.err)
				}
			} else {
				acceptedSinceSnapshot = int(engineDocument.Snapshot().Revision - result.revision)
			}
			if config.SnapshotOperationThreshold > 0 && acceptedSinceSnapshot >= config.SnapshotOperationThreshold {
				scheduleSnapshot()
			}
		case next := <-document.requests:
			switch next.kind {
			case requestSubmit:
				accepted, err := submit(ctx, engineDocument, value, next.operation, config.Telemetry)
				if err == nil {
					engineDocument = accepted.document
					if !accepted.duplicate {
						acceptedSinceSnapshot++
					}
				}
				next.response <- response{accepted: accepted.operation, duplicate: accepted.duplicate, err: err}
				if err == nil && !snapshotInFlight && config.SnapshotOperationThreshold > 0 && acceptedSinceSnapshot >= config.SnapshotOperationThreshold {
					scheduleSnapshot()
				}
			case requestSnapshot:
				next.response <- response{snapshot: engineDocument.Snapshot()}
			case requestBuildInverse:
				operation, err := engineDocument.BuildInverse(next.actorID, next.targetID, next.inverseID)
				next.response <- response{accepted: model.AcceptedOperation{Operation: operation}, err: err}
			default:
				next.response <- response{err: fmt.Errorf("unsupported document request %d", next.kind)}
			}
		}
	}
}

type snapshotResult struct {
	revision model.Revision
	err      error
}

func (document *Document) saveSnapshot(ctx context.Context, value store.SessionStore, snapshot model.Snapshot, results chan<- snapshotResult, telemetry *collabtelemetry.Telemetry) {
	document.snapshots.Add(1)
	go func() {
		defer document.snapshots.Done()
		storeContext, finishStore := observedStore(ctx, telemetry, collabtelemetry.StoreSnapshot)
		err := value.SaveSnapshot(storeContext, snapshot)
		finishStore(err)
		select {
		case results <- snapshotResult{revision: snapshot.Revision, err: err}:
		case <-ctx.Done():
		}
	}()
}

type submitResult struct {
	operation model.AcceptedOperation
	document  *engine.Document
	duplicate bool
}

func submit(ctx context.Context, document *engine.Document, value store.SessionStore, operation model.Operation, telemetry *collabtelemetry.Telemetry) (submitResult, error) {
	prior, exists, err := value.LookupOperation(ctx, operation.DocumentID, operation.OperationID)
	if err != nil {
		return submitResult{}, err
	}
	if exists {
		return submitResult{operation: prior, document: document, duplicate: true}, nil
	}
	candidate := document.Clone()
	accepted, err := candidate.Apply(operation, time.Now().UTC())
	if err != nil {
		return submitResult{}, err
	}
	storeContext, finishStore := observedStore(ctx, telemetry, collabtelemetry.StoreAppend)
	if err := value.Append(storeContext, accepted); err != nil {
		finishStore(err)
		return submitResult{}, err
	}
	finishStore(nil)
	return submitResult{operation: accepted, document: candidate}, nil
}

func observedStore(ctx context.Context, telemetry *collabtelemetry.Telemetry, operation collabtelemetry.StoreSignal) (context.Context, func(error)) {
	if telemetry == nil {
		return ctx, func(error) {}
	}
	return telemetry.Store(ctx, operation)
}
