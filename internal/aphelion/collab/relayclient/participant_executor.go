package relayclient

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"sdmm/internal/aphelion/collab/client"
	"sdmm/internal/aphelion/collab/model"
	legacyprotocol "sdmm/internal/aphelion/collab/protocol"
	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/store"
)

var errParticipantOperationRejected = errors.New("collaboration operation was rejected")

type participantResult struct {
	accepted model.AcceptedOperation
	err      error
}

type ParticipantExecutor struct {
	participant *Participant
	actorID     model.ActorID
	role        protocolv2.Role
	mutex       sync.Mutex
	projection  client.Projection
	pending     map[model.OperationID]chan participantResult
	accepted    map[model.OperationID]model.AcceptedOperation
	updates     chan client.Projection
	terminal    error
	start       sync.Once
}

func NewParticipantExecutor(participant *Participant, actorID model.ActorID) (*ParticipantExecutor, error) {
	if participant == nil || participant.Replica == nil || participant.Transport == nil {
		return nil, fmt.Errorf("participant executor is incomplete")
	}
	if err := actorID.Validate(); err != nil {
		return nil, err
	}
	snapshot, err := participant.Replica.Snapshot(context.Background())
	if err != nil {
		return nil, err
	}
	return &ParticipantExecutor{
		participant: participant, actorID: actorID, role: participant.Replica.LocalSession().Role, projection: client.NewProjection(snapshot),
		pending: make(map[model.OperationID]chan participantResult), accepted: make(map[model.OperationID]model.AcceptedOperation), updates: make(chan client.Projection, 1),
	}, nil
}

func (execution *ParticipantExecutor) Start(ctx context.Context) {
	execution.start.Do(func() { go execution.run(ctx) })
}

func (execution *ParticipantExecutor) Execute(ctx context.Context, operation model.Operation) (model.AcceptedOperation, error) {
	if err := ctx.Err(); err != nil {
		return model.AcceptedOperation{}, err
	}
	if execution.role == protocolv2.RoleViewer {
		return model.AcceptedOperation{}, fmt.Errorf("viewer sessions are read only")
	}
	operation.ActorID = execution.actorID
	if operation.OperationID == "" {
		operationID, err := model.NewOperationID()
		if err != nil {
			return model.AcceptedOperation{}, err
		}
		operation.OperationID = operationID
	}
	execution.mutex.Lock()
	if execution.terminal != nil {
		err := execution.terminal
		execution.mutex.Unlock()
		return model.AcceptedOperation{}, err
	}
	projection, err := execution.projection.Submit(operation)
	if err != nil {
		execution.mutex.Unlock()
		return model.AcceptedOperation{}, err
	}
	if err := execution.participant.Replica.SavePending(ctx, operation); err != nil {
		execution.mutex.Unlock()
		return model.AcceptedOperation{}, err
	}
	result := make(chan participantResult, 1)
	execution.projection = projection
	execution.pending[operation.OperationID] = result
	execution.publishLocked()
	execution.mutex.Unlock()
	if err := execution.participant.Submit(ctx, operation); err != nil {
		execution.fail(err)
		return model.AcceptedOperation{}, err
	}
	select {
	case resolved := <-result:
		return resolved.accepted, resolved.err
	case <-ctx.Done():
		return model.AcceptedOperation{}, ctx.Err()
	}
}

func (execution *ParticipantExecutor) ExecuteAsync(ctx context.Context, operation model.Operation, complete func(model.AcceptedOperation, error)) error {
	if complete == nil {
		return fmt.Errorf("participant executor completion callback is nil")
	}
	go func() {
		accepted, err := execution.Execute(ctx, operation)
		complete(accepted, err)
	}()
	return nil
}

func (execution *ParticipantExecutor) BuildInverse(ctx context.Context, targetID model.OperationID) (model.Operation, error) {
	if err := ctx.Err(); err != nil {
		return model.Operation{}, err
	}
	execution.mutex.Lock()
	defer execution.mutex.Unlock()
	target, exists := execution.accepted[targetID]
	if !exists || target.ActorID != execution.actorID || target.Kind == model.OperationKindInverse {
		return model.Operation{}, fmt.Errorf("accepted operation is not safely invertible by this actor")
	}
	operationID, err := model.NewOperationID()
	if err != nil {
		return model.Operation{}, err
	}
	baseHash, err := execution.projection.Acknowledged.Hash()
	if err != nil {
		return model.Operation{}, err
	}
	changes := make([]model.TileChange, len(target.Changes))
	for index, targetChange := range target.Changes {
		current := participantTileState(execution.projection.Acknowledged, targetChange.Coord)
		if !current.Equal(targetChange.After) {
			return model.Operation{}, fmt.Errorf("accepted operation is no longer safely reversible")
		}
		changes[index] = model.TileChange{Coord: targetChange.Coord, Before: model.CloneTileState(targetChange.After), After: model.CloneTileState(targetChange.Before)}
	}
	inverseOf := targetID
	return model.Operation{ProtocolVersion: model.ProtocolVersion, DocumentID: execution.projection.Acknowledged.DocumentID, ActorID: execution.actorID, OperationID: operationID, BaseRevision: execution.projection.Acknowledged.Revision, EnvironmentHash: execution.projection.Acknowledged.EnvironmentHash, BaseMapHash: baseHash, Kind: model.OperationKindInverse, Changes: changes, InverseOf: &inverseOf}, nil
}

func (execution *ParticipantExecutor) Snapshot(ctx context.Context) (model.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return model.Snapshot{}, err
	}
	execution.mutex.Lock()
	defer execution.mutex.Unlock()
	return model.CloneSnapshot(execution.projection.Acknowledged), nil
}

func (execution *ParticipantExecutor) ProjectionUpdates() <-chan client.Projection {
	return execution.updates
}

func (execution *ParticipantExecutor) HasUnacknowledgedOperations() bool {
	execution.mutex.Lock()
	defer execution.mutex.Unlock()
	return len(execution.pending) != 0
}

func (execution *ParticipantExecutor) TerminalError() error {
	execution.mutex.Lock()
	defer execution.mutex.Unlock()
	return execution.terminal
}

func (execution *ParticipantExecutor) run(ctx context.Context) {
	for {
		sender, message, err := execution.participant.Transport.ReadApplication(ctx, execution.participant.GroupKey)
		if err != nil {
			execution.fail(err)
			return
		}
		switch message.Type {
		case protocolv2.ApplicationOperationAccepted:
			accepted := *message.Payload.(*protocolv2.OperationAccepted)
			acknowledgement, err := execution.participant.Replica.ApplyAccepted(ctx, sender, accepted)
			if err != nil {
				execution.fail(err)
				return
			}
			execution.mutex.Lock()
			projection, err := execution.projection.Accept(accepted.Operation, accepted.MapHash)
			if err == nil {
				execution.projection = projection
				execution.accepted[accepted.Operation.OperationID] = accepted.Operation
				if waiter := execution.pending[accepted.Operation.OperationID]; waiter != nil {
					delete(execution.pending, accepted.Operation.OperationID)
					waiter <- participantResult{accepted: accepted.Operation}
				}
				execution.publishLocked()
			}
			execution.mutex.Unlock()
			if err != nil {
				execution.fail(err)
				return
			}
			if err := execution.participant.Transport.SendApplication(ctx, execution.participant.GroupKey, protocolv2.RouteOwner, protocolv2.ActorKey{}, protocolv2.ApplicationRevisionAcknowledged, acknowledgement); err != nil {
				execution.fail(err)
				return
			}
		case protocolv2.ApplicationOperationRejected:
			rejected := *message.Payload.(*protocolv2.OperationRejected)
			execution.mutex.Lock()
			projection, _, err := execution.projection.Reject(legacyprotocol.OperationRejectedPayload{OperationID: rejected.OperationID, Code: rejected.Code, Message: rejected.Message, Revision: rejected.Revision, MapHash: rejected.MapHash, AuthoritativeValues: rejected.AuthoritativeValues})
			if err == nil {
				execution.projection = projection
				if waiter := execution.pending[rejected.OperationID]; waiter != nil {
					delete(execution.pending, rejected.OperationID)
					waiter <- participantResult{err: fmt.Errorf("%w: %s", errParticipantOperationRejected, rejected.Code)}
				}
				execution.publishLocked()
			}
			execution.mutex.Unlock()
			if err != nil {
				execution.fail(err)
				return
			}
			_ = execution.participant.Replica.ResolvePending(ctx, rejected.OperationID, store.PendingConflicting, rejected.Code)
		}
	}
}

func (execution *ParticipantExecutor) publishLocked() {
	projection := execution.projection
	select {
	case execution.updates <- projection:
	default:
		select {
		case <-execution.updates:
		default:
		}
		execution.updates <- projection
	}
}

func (execution *ParticipantExecutor) fail(err error) {
	execution.mutex.Lock()
	defer execution.mutex.Unlock()
	if execution.terminal != nil {
		return
	}
	execution.terminal = err
	for operationID, waiter := range execution.pending {
		delete(execution.pending, operationID)
		waiter <- participantResult{err: err}
	}
}

func participantTileState(snapshot model.Snapshot, coord model.Coord) model.TileState {
	for _, tile := range snapshot.Tiles {
		if tile.Coord == coord {
			return model.CloneTileState(tile.State)
		}
	}
	return model.TileState{}
}
