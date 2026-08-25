package client

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/coder/websocket"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

func TestNetworkExecutorAcceptRejectAndInverse(t *testing.T) {
	t.Parallel()

	snapshot := projectionSnapshot(t)
	actorID, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	transport := newFakeTransport()
	network, err := NewNetworkExecutor(transport, snapshot, actorID, "session")
	if err != nil {
		t.Fatal(err)
	}
	operation := projectionOperation(t, snapshot, 1)
	result := make(chan model.AcceptedOperation, 1)
	errorsFound := make(chan error, 1)
	go func() {
		accepted, executeErr := network.Execute(context.Background(), operation)
		result <- accepted
		errorsFound <- executeErr
	}()
	submitted := transport.next(t)
	decoded, err := protocol.DecodeClient(mustJSON(t, submitted))
	if err != nil {
		t.Fatal(err)
	}
	submission := decoded.Payload.(*protocol.OperationSubmitPayload).Operation
	accepted := model.AcceptedOperation{Operation: submission, Revision: 1, AcceptedAt: time.Unix(1, 0)}
	acceptedSnapshot := snapshotWithOperation(t, snapshot, accepted)
	acceptedHash, err := acceptedSnapshot.Hash()
	if err != nil {
		t.Fatal(err)
	}
	network.Receive(serverEnvelope(t, protocol.ServerOperationAccepted, protocol.OperationAcceptedPayload{Operation: accepted, MapHash: acceptedHash}))
	if executeErr := <-errorsFound; executeErr != nil {
		t.Fatal(executeErr)
	}
	if actual := <-result; actual.Revision != 1 {
		t.Fatalf("Execute() revision = %d, want 1", actual.Revision)
	}
	synchronized, err := network.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if synchronized.Revision != 1 {
		t.Fatalf("Snapshot() revision = %d, want 1", synchronized.Revision)
	}
	inverse, err := network.BuildInverse(context.Background(), accepted.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if inverse.InverseOf == nil || *inverse.InverseOf != accepted.OperationID || inverse.ActorID != actorID {
		t.Fatalf("BuildInverse() = %#v", inverse)
	}

	rejectedOperation := projectionOperation(t, synchronized, 2)
	rejectedErrors := make(chan error, 1)
	go func() {
		_, executeErr := network.Execute(context.Background(), rejectedOperation)
		rejectedErrors <- executeErr
	}()
	rejectedSubmission := transport.next(t)
	rejectedDecoded, err := protocol.DecodeClient(mustJSON(t, rejectedSubmission))
	if err != nil {
		t.Fatal(err)
	}
	rejectedID := rejectedDecoded.Payload.(*protocol.OperationSubmitPayload).Operation.OperationID
	network.Receive(serverEnvelope(t, protocol.ServerOperationRejected, protocol.OperationRejectedPayload{OperationID: rejectedID, Code: "precondition_failed", Message: "conflict", Revision: 1, MapHash: acceptedHash}))
	if executeErr := <-rejectedErrors; !errors.Is(executeErr, ErrOperationRejected) {
		t.Fatalf("Execute(rejected) error = %v, want %v", executeErr, ErrOperationRejected)
	}
}

func TestNetworkExecutorConformanceOverWebSocket(t *testing.T) {
	t.Parallel()

	baseURL, sessionID, token, snapshot, shutdown := startClientTestService(t)
	defer shutdown()
	transport := NewWebSocketTransport(TransportConfig{})
	received := make(chan protocol.ServerEnvelope, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := transport.Connect(ctx, protocol.JoinRequest{BaseURL: baseURL, Origin: "http://127.0.0.1", Token: token, SessionID: sessionID}, func(message protocol.ServerEnvelope) {
		received <- message
	}); err != nil {
		t.Fatal(err)
	}
	joined := waitForServerType(t, received, protocol.ServerJoined)
	decodedJoined, err := protocol.DecodeServer(mustJSON(t, joined))
	if err != nil {
		t.Fatal(err)
	}
	actorID := decodedJoined.Payload.(*protocol.JoinedPayload).ActorID
	network, err := NewNetworkExecutor(transport, snapshot, actorID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	waitForServerType(t, received, protocol.ServerReplayComplete)

	operation := projectionOperation(t, snapshot, 1)
	accepted := executeWhileReceiving(t, network, received, operation)
	if accepted.Revision != 1 {
		t.Fatalf("Execute() revision = %d, want 1", accepted.Revision)
	}
	inverse, err := network.BuildInverse(context.Background(), accepted.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	acceptedInverse := executeWhileReceiving(t, network, received, inverse)
	if acceptedInverse.Revision != 2 {
		t.Fatalf("Execute(inverse) revision = %d, want 2", acceptedInverse.Revision)
	}
	result, err := network.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != 2 || !tileAt(result, 1).Equal(model.TileState{}) {
		t.Fatalf("Snapshot() = revision %d tile %#v, want revision 2 empty tile", result.Revision, tileAt(result, 1))
	}
}

func TestNetworkExecutorExecuteAsyncDoesNotWaitForAcknowledgement(t *testing.T) {
	t.Parallel()

	snapshot := projectionSnapshot(t)
	actorID, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	transport := newFakeTransport()
	network, err := NewNetworkExecutor(transport, snapshot, actorID, "session")
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan operationResult, 1)
	if err := network.ExecuteAsync(context.Background(), projectionOperation(t, snapshot, 1), func(accepted model.AcceptedOperation, executeErr error) {
		results <- operationResult{accepted: accepted, err: executeErr}
	}); err != nil {
		t.Fatal(err)
	}
	submitted := transport.next(t)
	select {
	case result := <-results:
		t.Fatalf("callback completed before acknowledgement: %#v", result)
	default:
	}
	decoded, err := protocol.DecodeClient(mustJSON(t, submitted))
	if err != nil {
		t.Fatal(err)
	}
	operation := decoded.Payload.(*protocol.OperationSubmitPayload).Operation
	accepted := model.AcceptedOperation{Operation: operation, Revision: 1, AcceptedAt: time.Unix(1, 0)}
	after := snapshotWithOperation(t, snapshot, accepted)
	mapHash, err := after.Hash()
	if err != nil {
		t.Fatal(err)
	}
	network.Receive(serverEnvelope(t, protocol.ServerOperationAccepted, protocol.OperationAcceptedPayload{Operation: accepted, MapHash: mapHash}))
	select {
	case result := <-results:
		if result.err != nil || result.accepted.Revision != 1 {
			t.Fatalf("callback result = %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("callback did not receive acknowledgement")
	}
}

func TestNetworkExecutorTerminationReleasesPendingOperation(t *testing.T) {
	t.Parallel()

	snapshot := projectionSnapshot(t)
	actorID, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	transport := newFakeTransport()
	network, err := NewNetworkExecutor(transport, snapshot, actorID, "session")
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 1)
	if err := network.ExecuteAsync(context.Background(), projectionOperation(t, snapshot, 1), func(_ model.AcceptedOperation, executeErr error) {
		results <- executeErr
	}); err != nil {
		t.Fatal(err)
	}
	transport.next(t)

	cause := errors.New("connection lost")
	network.Terminate(cause)
	select {
	case executeErr := <-results:
		if !errors.Is(executeErr, cause) {
			t.Fatalf("pending operation error = %v, want %v", executeErr, cause)
		}
	case <-time.After(time.Second):
		t.Fatal("pending operation remained blocked after executor termination")
	}
	after, err := network.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("read acknowledged snapshot after termination: %v", err)
	}
	if !tileAt(after, 1).Equal(tileAt(snapshot, 1)) {
		t.Fatal("terminated executor did not retain the acknowledged snapshot for rollback")
	}
	if network.HasUnacknowledgedOperations() {
		t.Fatal("terminated executor retained speculative operations")
	}
}

func executeWhileReceiving(t *testing.T, network *NetworkExecutor, received <-chan protocol.ServerEnvelope, operation model.Operation) model.AcceptedOperation {
	t.Helper()
	results := make(chan operationResult, 1)
	go func() {
		accepted, err := network.Execute(context.Background(), operation)
		results <- operationResult{accepted: accepted, err: err}
	}()
	deadline := time.After(time.Second)
	for {
		select {
		case message := <-received:
			network.Receive(message)
		case result := <-results:
			if result.err != nil {
				t.Fatal(result.err)
			}
			return result.accepted
		case <-deadline:
			t.Fatal("timed out waiting for network execution")
			return model.AcceptedOperation{}
		}
	}
}

type fakeTransport struct {
	sent chan protocol.ClientEnvelope
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{sent: make(chan protocol.ClientEnvelope, 8)}
}

func (transport *fakeTransport) Connect(context.Context, protocol.JoinRequest, func(protocol.ServerEnvelope)) error {
	return nil
}

func (transport *fakeTransport) Send(_ context.Context, envelope protocol.ClientEnvelope) error {
	transport.sent <- envelope
	return nil
}

func (transport *fakeTransport) Close(websocket.StatusCode, string) error { return nil }

func (transport *fakeTransport) next(t *testing.T) protocol.ClientEnvelope {
	t.Helper()
	select {
	case envelope := <-transport.sent:
		return envelope
	case <-time.After(time.Second):
		t.Fatal("transport received no message")
		return protocol.ClientEnvelope{}
	}
}

func serverEnvelope(t *testing.T, messageType protocol.ServerType, payload any) protocol.ServerEnvelope {
	t.Helper()
	return protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "server", SessionID: "session", Type: messageType, Payload: mustJSON(t, payload)}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
