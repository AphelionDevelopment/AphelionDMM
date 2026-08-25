package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	collabtelemetry "sdmm/internal/aphelion/collab/telemetry"
)

const documentRequestQueueSize = 256

var ErrDocumentClosed = errors.New("document owner is closed")

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

type DocumentOwner struct {
	requests  chan request
	done      chan struct{}
	cancel    context.CancelFunc
	closeOnce sync.Once
	snapshots sync.WaitGroup
}

func StartDocument(ctx context.Context, snapshot model.Snapshot, store SessionStore) (*DocumentOwner, error) {
	return StartDocumentWithConfig(ctx, snapshot, store, DocumentConfig{})
}

func StartDocumentWithConfig(ctx context.Context, snapshot model.Snapshot, store SessionStore, config DocumentConfig) (*DocumentOwner, error) {
	if store == nil {
		return nil, fmt.Errorf("start document: store is nil")
	}
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		return nil, fmt.Errorf("start document: %w", err)
	}
	storeContext := ctx
	finishStore := func(error) {}
	if config.Telemetry != nil {
		storeContext, finishStore = config.Telemetry.Store(ctx, collabtelemetry.StoreCreate)
	}
	if err := store.Create(storeContext, snapshot); err != nil {
		finishStore(err)
		return nil, fmt.Errorf("create stored session: %w", err)
	}
	finishStore(nil)
	return startDocument(ctx, document, store, config), nil
}

func startDocument(ctx context.Context, document *engine.Document, store SessionStore, config DocumentConfig) *DocumentOwner {
	runContext, cancel := context.WithCancel(ctx)
	owner := &DocumentOwner{
		requests: make(chan request, documentRequestQueueSize),
		done:     make(chan struct{}),
		cancel:   cancel,
	}
	go owner.run(runContext, document, store, config)
	return owner
}

func (owner *DocumentOwner) Submit(ctx context.Context, operation model.Operation) (model.AcceptedOperation, error) {
	accepted, _, err := owner.SubmitWithStatus(ctx, operation)
	return accepted, err
}

func (owner *DocumentOwner) SubmitWithStatus(ctx context.Context, operation model.Operation) (model.AcceptedOperation, bool, error) {
	result, err := owner.request(ctx, request{kind: requestSubmit, operation: model.CloneOperation(operation)})
	return result.accepted, result.duplicate, err
}

func (owner *DocumentOwner) Snapshot(ctx context.Context) (model.Snapshot, error) {
	result, err := owner.request(ctx, request{kind: requestSnapshot})
	return result.snapshot, err
}

func (owner *DocumentOwner) BuildInverse(ctx context.Context, actorID model.ActorID, targetID, inverseID model.OperationID) (model.Operation, error) {
	result, err := owner.request(ctx, request{kind: requestBuildInverse, actorID: actorID, targetID: targetID, inverseID: inverseID})
	return result.accepted.Operation, err
}

func (owner *DocumentOwner) Close(ctx context.Context) error {
	owner.closeOnce.Do(owner.cancel)
	select {
	case <-owner.done:
		owner.snapshots.Wait()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (owner *DocumentOwner) request(ctx context.Context, value request) (response, error) {
	value.response = make(chan response, 1)
	select {
	case owner.requests <- value:
	case <-owner.done:
		return response{}, ErrDocumentClosed
	case <-ctx.Done():
		return response{}, ctx.Err()
	}
	select {
	case result := <-value.response:
		return result, result.err
	case <-owner.done:
		return response{}, ErrDocumentClosed
	case <-ctx.Done():
		return response{}, ctx.Err()
	}
}

func (owner *DocumentOwner) run(ctx context.Context, document *engine.Document, store SessionStore, config DocumentConfig) {
	defer close(owner.done)
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
		owner.saveSnapshot(ctx, store, document.Snapshot(), snapshotResults, config.Telemetry)
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
				acceptedSinceSnapshot = int(document.Snapshot().Revision - result.revision)
			}
			if config.SnapshotOperationThreshold > 0 && acceptedSinceSnapshot >= config.SnapshotOperationThreshold {
				scheduleSnapshot()
			}
		case request := <-owner.requests:
			switch request.kind {
			case requestSubmit:
				accepted, err := submit(ctx, document, store, request.operation, config.Telemetry)
				if err == nil {
					document = accepted.document
					if !accepted.duplicate {
						acceptedSinceSnapshot++
					}
				}
				request.response <- response{accepted: accepted.operation, duplicate: accepted.duplicate, err: err}
				if err == nil && !snapshotInFlight && config.SnapshotOperationThreshold > 0 && acceptedSinceSnapshot >= config.SnapshotOperationThreshold {
					scheduleSnapshot()
				}
			case requestSnapshot:
				request.response <- response{snapshot: document.Snapshot()}
			case requestBuildInverse:
				operation, err := document.BuildInverse(request.actorID, request.targetID, request.inverseID)
				request.response <- response{accepted: model.AcceptedOperation{Operation: operation}, err: err}
			default:
				request.response <- response{err: fmt.Errorf("unsupported document request %d", request.kind)}
			}
		}
	}
}

type snapshotResult struct {
	revision model.Revision
	err      error
}

func (owner *DocumentOwner) saveSnapshot(ctx context.Context, store SessionStore, snapshot model.Snapshot, results chan<- snapshotResult, observability *collabtelemetry.Telemetry) {
	owner.snapshots.Add(1)
	go func() {
		defer owner.snapshots.Done()
		storeContext := ctx
		finishStore := func(error) {}
		if observability != nil {
			storeContext, finishStore = observability.Store(ctx, collabtelemetry.StoreSnapshot)
		}
		err := store.SaveSnapshot(storeContext, snapshot)
		finishStore(err)
		result := snapshotResult{revision: snapshot.Revision, err: err}
		select {
		case results <- result:
		case <-ctx.Done():
		}
	}()
}

type submitResult struct {
	operation model.AcceptedOperation
	document  *engine.Document
	duplicate bool
}

func submit(ctx context.Context, document *engine.Document, store SessionStore, operation model.Operation, observability *collabtelemetry.Telemetry) (submitResult, error) {
	prior, exists, err := store.LookupOperation(ctx, operation.DocumentID, operation.OperationID)
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
	storeContext := ctx
	finishStore := func(error) {}
	if observability != nil {
		storeContext, finishStore = observability.Store(ctx, collabtelemetry.StoreAppend)
	}
	if err := store.Append(storeContext, accepted); err != nil {
		finishStore(err)
		return submitResult{}, err
	}
	finishStore(nil)
	return submitResult{operation: accepted, document: candidate}, nil
}
