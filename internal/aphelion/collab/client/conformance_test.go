package client

import (
	"context"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/executor"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

func TestLocalAndNetworkExecutorsShareConformance(t *testing.T) {
	t.Parallel()

	factories := map[string]func(*testing.T, model.Snapshot, model.ActorID) executor.Executor{
		"local": func(t *testing.T, snapshot model.Snapshot, actorID model.ActorID) executor.Executor {
			t.Helper()
			document, err := engine.NewDocument(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			local, err := executor.NewLocal(document, actorID)
			if err != nil {
				t.Fatal(err)
			}
			return local
		},
		"network": func(t *testing.T, snapshot model.Snapshot, actorID model.ActorID) executor.Executor {
			t.Helper()
			transport := newFakeTransport()
			network, err := NewNetworkExecutor(transport, snapshot, actorID, "session")
			if err != nil {
				t.Fatal(err)
			}
			return &conformanceNetworkExecutor{t: t, transport: transport, network: network}
		},
	}
	for name, factory := range factories {
		name, factory := name, factory
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			snapshot := projectionSnapshot(t)
			actorID, err := model.NewActorID()
			if err != nil {
				t.Fatal(err)
			}
			execution := factory(t, snapshot, actorID)

			accepted, err := execution.Execute(context.Background(), projectionOperation(t, snapshot, 1))
			if err != nil {
				t.Fatalf("Execute(forward): %v", err)
			}
			if accepted.Revision != 1 || accepted.ActorID != actorID {
				t.Fatalf("Execute(forward) = revision %d actor %q, want revision 1 actor %q", accepted.Revision, accepted.ActorID, actorID)
			}
			inverse, err := execution.BuildInverse(context.Background(), accepted.OperationID)
			if err != nil {
				t.Fatalf("BuildInverse(): %v", err)
			}
			if inverse.InverseOf == nil || *inverse.InverseOf != accepted.OperationID {
				t.Fatalf("BuildInverse() target = %v, want %q", inverse.InverseOf, accepted.OperationID)
			}
			acceptedInverse, err := execution.Execute(context.Background(), inverse)
			if err != nil {
				t.Fatalf("Execute(inverse): %v", err)
			}
			result, err := execution.Snapshot(context.Background())
			if err != nil {
				t.Fatalf("Snapshot(): %v", err)
			}
			if acceptedInverse.Revision != 2 || result.Revision != 2 || !tileAt(result, 1).Equal(model.TileState{}) {
				t.Fatalf("inverse result = accepted revision %d snapshot revision %d tile %#v", acceptedInverse.Revision, result.Revision, tileAt(result, 1))
			}

			canceled, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := execution.Snapshot(canceled); err == nil {
				t.Fatal("Snapshot(canceled) error = nil")
			}
		})
	}
}

type conformanceNetworkExecutor struct {
	t         *testing.T
	transport *fakeTransport
	network   *NetworkExecutor
}

func (execution *conformanceNetworkExecutor) Execute(ctx context.Context, operation model.Operation) (model.AcceptedOperation, error) {
	type result struct {
		accepted model.AcceptedOperation
		err      error
	}
	results := make(chan result, 1)
	go func() {
		accepted, err := execution.network.Execute(ctx, operation)
		results <- result{accepted: accepted, err: err}
	}()
	submitted := execution.transport.next(execution.t)
	decoded, err := protocol.DecodeClient(mustJSON(execution.t, submitted))
	if err != nil {
		execution.t.Fatal(err)
	}
	submission := decoded.Payload.(*protocol.OperationSubmitPayload).Operation
	current, err := execution.network.Snapshot(context.Background())
	if err != nil {
		execution.t.Fatal(err)
	}
	accepted := model.AcceptedOperation{Operation: submission, Revision: current.Revision + 1, AcceptedAt: time.Unix(int64(current.Revision+1), 0)}
	after := snapshotWithOperation(execution.t, current, accepted)
	mapHash, err := after.Hash()
	if err != nil {
		execution.t.Fatal(err)
	}
	if err := execution.network.Receive(serverEnvelope(execution.t, protocol.ServerOperationAccepted, protocol.OperationAcceptedPayload{Operation: accepted, MapHash: mapHash})); err != nil {
		execution.t.Fatal(err)
	}
	resolved := <-results
	return resolved.accepted, resolved.err
}

func (execution *conformanceNetworkExecutor) BuildInverse(ctx context.Context, operationID model.OperationID) (model.Operation, error) {
	return execution.network.BuildInverse(ctx, operationID)
}

func (execution *conformanceNetworkExecutor) Snapshot(ctx context.Context) (model.Snapshot, error) {
	return execution.network.Snapshot(ctx)
}
