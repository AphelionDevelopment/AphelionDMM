package relayclient

import (
	"context"
	"fmt"

	"sdmm/internal/aphelion/collab/authority"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
)

type OwnerExecutor struct {
	transport *Client
	authority *authority.OwnerSession
	groupKey  protocolv2.GroupKey
}

func NewOwnerExecutor(transport *Client, ownerAuthority *authority.OwnerSession, groupKey protocolv2.GroupKey) (*OwnerExecutor, error) {
	if transport == nil || ownerAuthority == nil || groupKey == (protocolv2.GroupKey{}) {
		return nil, fmt.Errorf("owner executor is incomplete")
	}
	return &OwnerExecutor{transport: transport, authority: ownerAuthority, groupKey: groupKey}, nil
}

func (execution *OwnerExecutor) Execute(ctx context.Context, operation model.Operation) (model.AcceptedOperation, error) {
	accepted, err := execution.authority.ExecuteLocal(ctx, execution.transport.ActorKey(), operation)
	if err != nil {
		return model.AcceptedOperation{}, err
	}
	if err := execution.transport.SendApplication(ctx, execution.groupKey, protocolv2.RouteRoom, protocolv2.ActorKey{}, protocolv2.ApplicationOperationAccepted, accepted); err != nil {
		return accepted.Operation, fmt.Errorf("broadcast committed owner operation: %w", err)
	}
	return accepted.Operation, nil
}

func (execution *OwnerExecutor) ExecuteAsync(ctx context.Context, operation model.Operation, complete func(model.AcceptedOperation, error)) error {
	if complete == nil {
		return fmt.Errorf("owner executor completion callback is nil")
	}
	go func() {
		accepted, err := execution.Execute(ctx, operation)
		complete(accepted, err)
	}()
	return nil
}

func (execution *OwnerExecutor) BuildInverse(ctx context.Context, targetID model.OperationID) (model.Operation, error) {
	return execution.authority.BuildLocalInverse(ctx, execution.transport.ActorKey(), targetID)
}

func (execution *OwnerExecutor) Snapshot(ctx context.Context) (model.Snapshot, error) {
	return execution.authority.Snapshot(ctx)
}

func (execution *OwnerExecutor) HasUnacknowledgedOperations() bool {
	return false
}
