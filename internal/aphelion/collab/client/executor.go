package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

var (
	ErrOperationRejected  = errors.New("collaboration operation was rejected")
	ErrExecutorTerminated = errors.New("collaboration network executor was terminated")
)

const maxRetainedConflicts = 100

type operationResult struct {
	accepted model.AcceptedOperation
	err      error
}

type NetworkExecutor struct {
	transport Transport
	actor     model.ActorID
	sessionID string

	mutex      sync.Mutex
	projection Projection
	pending    map[model.OperationID]chan operationResult
	accepted   map[model.OperationID]model.AcceptedOperation
	conflicts  []Conflict
	updates    chan Projection
	terminal   error
}

func NewNetworkExecutor(transport Transport, snapshot model.Snapshot, actor model.ActorID, sessionID string) (*NetworkExecutor, error) {
	if transport == nil {
		return nil, fmt.Errorf("network executor transport is nil")
	}
	if _, err := snapshot.Hash(); err != nil {
		return nil, err
	}
	if err := actor.Validate(); err != nil {
		return nil, err
	}
	if sessionID == "" {
		return nil, fmt.Errorf("network executor session id is empty")
	}
	return &NetworkExecutor{
		transport:  transport,
		actor:      actor,
		sessionID:  sessionID,
		projection: NewProjection(snapshot),
		pending:    make(map[model.OperationID]chan operationResult),
		accepted:   make(map[model.OperationID]model.AcceptedOperation),
		updates:    make(chan Projection, 1),
	}, nil
}

func (network *NetworkExecutor) Execute(ctx context.Context, operation model.Operation) (model.AcceptedOperation, error) {
	if err := ctx.Err(); err != nil {
		return model.AcceptedOperation{}, err
	}
	operation.ActorID = network.actor
	if operation.OperationID == "" {
		operationID, err := model.NewOperationID()
		if err != nil {
			return model.AcceptedOperation{}, err
		}
		operation.OperationID = operationID
	}

	network.mutex.Lock()
	if network.terminal != nil {
		err := network.terminal
		network.mutex.Unlock()
		return model.AcceptedOperation{}, err
	}
	projection, err := network.projection.Submit(operation)
	if err != nil {
		network.mutex.Unlock()
		return model.AcceptedOperation{}, err
	}
	result := make(chan operationResult, 1)
	network.projection = projection
	network.pending[operation.OperationID] = result
	network.publishLocked()
	network.mutex.Unlock()

	payload, err := json.Marshal(protocol.OperationSubmitPayload{Operation: operation})
	if err != nil {
		network.failPending(operation.OperationID, err)
		return model.AcceptedOperation{}, err
	}
	envelope := protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: string(operation.OperationID), SessionID: network.sessionID, Type: protocol.ClientOperationSubmit, Payload: payload}
	if err := network.transport.Send(ctx, envelope); err != nil {
		network.failPending(operation.OperationID, err)
		return model.AcceptedOperation{}, err
	}
	select {
	case resolved := <-result:
		return resolved.accepted, resolved.err
	case <-ctx.Done():
		return model.AcceptedOperation{}, ctx.Err()
	}
}

func (network *NetworkExecutor) ExecuteAsync(ctx context.Context, operation model.Operation, complete func(model.AcceptedOperation, error)) error {
	if complete == nil {
		return fmt.Errorf("network executor completion callback is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	go func() {
		accepted, err := network.Execute(ctx, operation)
		complete(accepted, err)
	}()
	return nil
}

func (network *NetworkExecutor) BuildInverse(ctx context.Context, targetID model.OperationID) (model.Operation, error) {
	if err := ctx.Err(); err != nil {
		return model.Operation{}, err
	}
	network.mutex.Lock()
	defer network.mutex.Unlock()
	target, exists := network.accepted[targetID]
	if !exists {
		return model.Operation{}, fmt.Errorf("accepted operation %q is not retained", targetID)
	}
	if target.ActorID != network.actor {
		return model.Operation{}, fmt.Errorf("accepted operation belongs to actor %q", target.ActorID)
	}
	if target.Kind == model.OperationKindInverse {
		return model.Operation{}, fmt.Errorf("inverse operations are redone as new forward operations")
	}
	operationID, err := model.NewOperationID()
	if err != nil {
		return model.Operation{}, err
	}
	baseHash, err := network.projection.Acknowledged.Hash()
	if err != nil {
		return model.Operation{}, err
	}
	changes := make([]model.TileChange, len(target.Changes))
	for index, targetChange := range target.Changes {
		current := tileStateAt(network.projection.Acknowledged, targetChange.Coord)
		if !current.Equal(targetChange.After) {
			return model.Operation{}, fmt.Errorf("accepted operation is no longer safely reversible at (%d,%d,%d)", targetChange.Coord.X, targetChange.Coord.Y, targetChange.Coord.Z)
		}
		changes[index] = model.TileChange{Coord: targetChange.Coord, Before: model.CloneTileState(targetChange.After), After: model.CloneTileState(targetChange.Before)}
	}
	inverseOf := targetID
	return model.Operation{ProtocolVersion: model.ProtocolVersion, DocumentID: network.projection.Acknowledged.DocumentID, ActorID: network.actor, OperationID: operationID, BaseRevision: network.projection.Acknowledged.Revision, EnvironmentHash: network.projection.Acknowledged.EnvironmentHash, BaseMapHash: baseHash, Kind: model.OperationKindInverse, Changes: changes, InverseOf: &inverseOf}, nil
}

func (network *NetworkExecutor) Snapshot(ctx context.Context) (model.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return model.Snapshot{}, err
	}
	network.mutex.Lock()
	defer network.mutex.Unlock()
	return model.CloneSnapshot(network.projection.Acknowledged), nil
}

func (network *NetworkExecutor) ProjectionUpdates() <-chan Projection {
	return network.updates
}

func (network *NetworkExecutor) HasUnacknowledgedOperations() bool {
	network.mutex.Lock()
	defer network.mutex.Unlock()
	return len(network.projection.Pending) != 0
}

func (network *NetworkExecutor) Conflicts() []Conflict {
	network.mutex.Lock()
	defer network.mutex.Unlock()
	conflicts := make([]Conflict, len(network.conflicts))
	for index, conflict := range network.conflicts {
		conflicts[index] = cloneConflict(conflict)
	}
	return conflicts
}

// Terminate releases pending operations and prevents further executor use.
func (network *NetworkExecutor) Terminate(cause error) {
	if cause == nil {
		cause = ErrExecutorTerminated
	}
	network.failAll(cause)
}

func (network *NetworkExecutor) Receive(envelope protocol.ServerEnvelope) {
	data, err := json.Marshal(envelope)
	if err != nil {
		network.failAll(err)
		return
	}
	decoded, err := protocol.DecodeServer(data)
	if err != nil {
		network.failAll(fmt.Errorf("decode network executor message: %w", err))
		return
	}
	network.mutex.Lock()
	defer network.mutex.Unlock()
	switch decoded.Envelope.Type {
	case protocol.ServerOperationAccepted:
		payload := decoded.Payload.(*protocol.OperationAcceptedPayload)
		projection, applyErr := network.projection.Accept(payload.Operation, payload.MapHash)
		if applyErr != nil {
			network.failAllLocked(applyErr)
			return
		}
		network.projection = projection
		network.accepted[payload.Operation.OperationID] = model.CloneAcceptedOperation(payload.Operation)
		if waiter, exists := network.pending[payload.Operation.OperationID]; exists {
			delete(network.pending, payload.Operation.OperationID)
			waiter <- operationResult{accepted: model.CloneAcceptedOperation(payload.Operation)}
		}
		network.publishLocked()
	case protocol.ServerOperationRejected:
		payload := decoded.Payload.(*protocol.OperationRejectedPayload)
		projection, conflict, rejectErr := network.projection.Reject(*payload)
		if rejectErr != nil {
			network.failAllLocked(rejectErr)
			return
		}
		network.projection = projection
		network.conflicts = append(network.conflicts, cloneConflict(conflict))
		if len(network.conflicts) > maxRetainedConflicts {
			network.conflicts = network.conflicts[len(network.conflicts)-maxRetainedConflicts:]
		}
		if waiter, exists := network.pending[payload.OperationID]; exists {
			delete(network.pending, payload.OperationID)
			waiter <- operationResult{err: fmt.Errorf("%w: %s: %s", ErrOperationRejected, conflict.Code, conflict.Message)}
		}
		network.publishLocked()
	}
}

func cloneConflict(conflict Conflict) Conflict {
	conflict.AuthoritativeValues = cloneTiles(conflict.AuthoritativeValues)
	return conflict
}

func (network *NetworkExecutor) failPending(operationID model.OperationID, cause error) {
	network.mutex.Lock()
	defer network.mutex.Unlock()
	if waiter, exists := network.pending[operationID]; exists {
		delete(network.pending, operationID)
		remaining := make([]model.Operation, 0, len(network.projection.Pending)-1)
		for _, operation := range network.projection.Pending {
			if operation.OperationID != operationID {
				remaining = append(remaining, model.CloneOperation(operation))
			}
		}
		rebased, err := rebasePending(network.projection.Acknowledged, remaining)
		if err != nil {
			waiter <- operationResult{err: cause}
			network.failAllLocked(fmt.Errorf("reapply pending operations after send failure: %w", err))
			return
		}
		network.projection.Pending = rebased
		waiter <- operationResult{err: cause}
		network.publishLocked()
	}
}

func (network *NetworkExecutor) failAll(cause error) {
	network.mutex.Lock()
	defer network.mutex.Unlock()
	network.failAllLocked(cause)
}

func (network *NetworkExecutor) failAllLocked(cause error) {
	if network.terminal != nil {
		return
	}
	network.terminal = cause
	for operationID, waiter := range network.pending {
		delete(network.pending, operationID)
		waiter <- operationResult{err: cause}
	}
	network.projection.Pending = nil
	network.publishLocked()
}

func (network *NetworkExecutor) publishLocked() {
	update := cloneProjection(network.projection)
	select {
	case network.updates <- update:
	default:
		select {
		case <-network.updates:
		default:
		}
		select {
		case network.updates <- update:
		default:
		}
	}
}

func tileStateAt(snapshot model.Snapshot, coord model.Coord) model.TileState {
	for _, tile := range snapshot.Tiles {
		if tile.Coord == coord {
			return model.CloneTileState(tile.State)
		}
	}
	return model.TileState{}
}
