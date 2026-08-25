package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

func TestTwoClientsConverge(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 3)
	service := NewService(ServiceConfig{AllowedOrigins: []string{"http://127.0.0.1"}})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	launchToken, err := service.NewLaunchToken()
	if err != nil {
		t.Fatal(err)
	}
	testServer := httptest.NewServer(service.Handler())
	defer testServer.Close()
	created := createTestSession(t, testServer.URL, launchToken, snapshot)
	editorToken := createTestJoinToken(t, testServer.URL, created.SessionID, created.OwnerToken, RoleEditor, "Editor")

	clientA := connectTestClient(t, testServer.URL, created.SessionID, created.OwnerToken, 0)
	defer func() { _ = clientA.CloseNow() }()
	clientB := connectTestClient(t, testServer.URL, created.SessionID, editorToken, 0)

	documentA, err := engine.NewDocument(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	documentB, err := engine.NewDocument(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	operationOne := testOperation(t, snapshot, 1)
	submitTestOperation(t, clientA, created.SessionID, "operation-1", operationOne)
	acceptedOneA := readAccepted(t, clientA)
	acceptedOneB := readAccepted(t, clientB)
	applyAccepted(t, documentA, acceptedOneA)
	applyAccepted(t, documentB, acceptedOneB)

	operationTwo := testOperation(t, snapshot, 2)
	submitTestOperation(t, clientB, created.SessionID, "operation-2", operationTwo)
	acceptedTwoA := readAccepted(t, clientA)
	acceptedTwoB := readAccepted(t, clientB)
	applyAccepted(t, documentA, acceptedTwoA)
	applyAccepted(t, documentB, acceptedTwoB)

	conflict := testOperation(t, snapshot, 1)
	submitTestOperation(t, clientB, created.SessionID, "conflict", conflict)
	rejected := readServerEnvelopeType(t, clientB, protocol.ServerOperationRejected)
	if rejected.Envelope.Type != protocol.ServerOperationRejected {
		t.Fatal("stale conflict was not rejected")
	}
	rejection := rejected.Payload.(*protocol.OperationRejectedPayload)
	if rejection.Code != "precondition_failed" {
		t.Fatalf("rejection code = %q, want precondition_failed", rejection.Code)
	}
	if len(rejection.AuthoritativeValues) != 1 || rejection.AuthoritativeValues[0].Coord.X != 1 || !rejection.AuthoritativeValues[0].State.Equal(acceptedOneA.Changes[0].After) {
		t.Fatalf("rejection authoritative values = %#v", rejection.AuthoritativeValues)
	}

	submitTestOperation(t, clientA, created.SessionID, "duplicate", operationOne)
	duplicate := readAccepted(t, clientA)
	if duplicate.Revision != acceptedOneA.Revision || duplicate.OperationID != acceptedOneA.OperationID {
		t.Fatalf("duplicate result = revision %d operation %q, want revision %d operation %q", duplicate.Revision, duplicate.OperationID, acceptedOneA.Revision, acceptedOneA.OperationID)
	}

	if err := clientB.Close(websocket.StatusNormalClosure, "reconnect test"); err != nil {
		t.Fatal(err)
	}
	for sequence := uint64(1); sequence <= 100; sequence++ {
		writeClientEnvelope(t, clientA, protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "presence", SessionID: created.SessionID, Type: protocol.ClientPresenceUpdate}, protocol.PresenceUpdatePayload{Sequence: sequence, Status: "active"})
	}
	operationThree := testOperation(t, snapshot, 3)
	submitTestOperation(t, clientA, created.SessionID, "operation-3", operationThree)
	acceptedThreeA := readAccepted(t, clientA)
	applyAccepted(t, documentA, acceptedThreeA)

	clientB = connectTestClient(t, testServer.URL, created.SessionID, editorToken, acceptedTwoB.Revision)
	defer func() { _ = clientB.CloseNow() }()
	replayedThree := readAccepted(t, clientB)
	if replayedThree.OperationID != acceptedThreeA.OperationID || replayedThree.Revision != acceptedThreeA.Revision {
		t.Fatalf("replayed operation = %#v, want %#v", replayedThree, acceptedThreeA)
	}
	applyAccepted(t, documentB, replayedThree)
	_ = readServerEnvelopeType(t, clientB, protocol.ServerReplayComplete)
	writeClientEnvelope(t, clientA, protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "inverse-3", SessionID: created.SessionID, Type: protocol.ClientInverseRequest}, protocol.InverseRequestPayload{OperationID: acceptedThreeA.OperationID})
	inverseA := readAccepted(t, clientA)
	inverseB := readAccepted(t, clientB)
	if inverseA.Kind != model.OperationKindInverse || inverseA.InverseOf == nil || *inverseA.InverseOf != acceptedThreeA.OperationID {
		t.Fatalf("inverse result = %#v", inverseA)
	}
	applyAccepted(t, documentA, inverseA)
	applyAccepted(t, documentB, inverseB)

	hashA, err := documentA.Snapshot().Hash()
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := documentB.Snapshot().Hash()
	if err != nil {
		t.Fatal(err)
	}
	serverSnapshot, err := service.sessions[created.SessionID].owner.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	serverHash, err := serverSnapshot.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if hashA != hashB || hashA != serverHash || serverSnapshot.Revision != 4 {
		t.Fatalf("convergence failed: A=%s B=%s server=%s revision=%d", hashA, hashB, serverHash, serverSnapshot.Revision)
	}
}

func createTestSession(t *testing.T, baseURL, launchToken string, snapshot model.Snapshot) CreateSessionResponse {
	t.Helper()
	body, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		t.Fatal(err)
	}
	response := postJSON(t, baseURL+"/v1/sessions", launchToken, body)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d", response.StatusCode)
	}
	var created CreateSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	return created
}

func createTestJoinToken(t *testing.T, baseURL, sessionID, ownerToken string, role Role, displayName string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"role": role, "display_name": displayName})
	if err != nil {
		t.Fatal(err)
	}
	response := postJSON(t, baseURL+"/v1/sessions/"+sessionID+"/join-tokens", ownerToken, body)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create join token status = %d", response.StatusCode)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result.Token
}

func connectTestClient(t *testing.T, baseURL, sessionID, token string, acknowledged model.Revision) *websocket.Conn {
	t.Helper()
	websocketURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/v1/collaboration"
	connection, _, err := websocket.Dial(context.Background(), websocketURL, &websocket.DialOptions{
		HTTPHeader:   http.Header{"Authorization": []string{"Bearer " + token}, "Origin": []string{"http://127.0.0.1"}},
		Subprotocols: []string{WebSocketSubprotocol},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeClientEnvelope(t, connection, protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "join", SessionID: sessionID, Type: protocol.ClientJoin}, protocol.JoinPayload{JoinToken: token, AcknowledgedRevision: acknowledged})
	if joined := readServerEnvelope(t, connection); joined.Envelope.Type != protocol.ServerJoined {
		t.Fatalf("first message = %q, want joined", joined.Envelope.Type)
	}
	if acknowledged == 0 {
		_ = readServerEnvelopeType(t, connection, protocol.ServerReplayComplete)
		_ = readServerEnvelopeType(t, connection, protocol.ServerPresenceSnapshot)
	}
	return connection
}

func submitTestOperation(t *testing.T, connection *websocket.Conn, sessionID, messageID string, operation model.Operation) {
	t.Helper()
	writeClientEnvelope(t, connection, protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: messageID, SessionID: sessionID, Type: protocol.ClientOperationSubmit}, protocol.OperationSubmitPayload{Operation: operation})
}

func readAccepted(t *testing.T, connection *websocket.Conn) model.AcceptedOperation {
	t.Helper()
	message := readServerEnvelopeType(t, connection, protocol.ServerOperationAccepted)
	return message.Payload.(*protocol.OperationAcceptedPayload).Operation
}

func applyAccepted(t *testing.T, document *engine.Document, accepted model.AcceptedOperation) {
	t.Helper()
	result, err := document.Apply(accepted.Operation, accepted.AcceptedAt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != accepted.Revision {
		t.Fatalf("local accepted revision = %d, want %d", result.Revision, accepted.Revision)
	}
}

func TestWebSocketReadLimitClosesOversizedMessage(t *testing.T) {
	t.Parallel()

	_, created, testServer := startHTTPTestSession(t)
	connection := connectTestClient(t, testServer.URL, created.SessionID, created.OwnerToken, 0)
	defer func() { _ = connection.CloseNow() }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := connection.Write(ctx, websocket.MessageText, []byte(strings.Repeat("x", protocol.MaxMessageBytes+1))); err != nil {
		t.Fatal(err)
	}
	_, _, err := connection.Read(ctx)
	if err == nil {
		t.Fatal("oversized WebSocket message did not close connection")
	}
}
