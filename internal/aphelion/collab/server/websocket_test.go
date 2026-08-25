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

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

func TestWebSocketRejectsOriginAndNegotiatesProtocol(t *testing.T) {
	t.Parallel()

	service, created, testServer := startHTTPTestSession(t)
	_ = service
	websocketURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/v1/collaboration"

	_, response, err := websocket.Dial(context.Background(), websocketURL, &websocket.DialOptions{
		HTTPHeader:   http.Header{"Authorization": []string{"Bearer " + created.OwnerToken}, "Origin": []string{"https://forbidden.example"}},
		Subprotocols: []string{WebSocketSubprotocol},
	})
	if err == nil {
		t.Fatal("forbidden origin WebSocket dial succeeded")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("forbidden origin status = %#v, want 403", response)
	}

	_, response, err = websocket.Dial(context.Background(), websocketURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + created.OwnerToken}, "Origin": []string{"http://127.0.0.1"}},
	})
	if err == nil {
		t.Fatal("missing subprotocol WebSocket dial succeeded")
	}
	if response == nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing subprotocol status = %#v, want 400", response)
	}
}

func TestWebSocketJoinPingPongAndGracefulClose(t *testing.T) {
	t.Parallel()

	_, created, testServer := startHTTPTestSession(t)
	websocketURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/v1/collaboration"
	connection, _, err := websocket.Dial(context.Background(), websocketURL, &websocket.DialOptions{
		HTTPHeader:   http.Header{"Authorization": []string{"Bearer " + created.OwnerToken}, "Origin": []string{"http://127.0.0.1"}},
		Subprotocols: []string{WebSocketSubprotocol},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.CloseNow() }()

	writeClientEnvelope(t, connection, protocol.ClientEnvelope{
		ProtocolVersion: model.ProtocolVersion,
		MessageID:       "join-1",
		SessionID:       created.SessionID,
		Type:            protocol.ClientJoin,
	}, protocol.JoinPayload{JoinToken: created.OwnerToken})
	joined := readServerEnvelope(t, connection)
	if joined.Envelope.Type != protocol.ServerJoined {
		t.Fatalf("first server message = %q, want joined", joined.Envelope.Type)
	}

	writeClientEnvelope(t, connection, protocol.ClientEnvelope{
		ProtocolVersion: model.ProtocolVersion,
		MessageID:       "ping-1",
		SessionID:       created.SessionID,
		Type:            protocol.ClientPing,
	}, protocol.PingPayload{Nonce: "nonce-1"})
	pong := readServerEnvelopeType(t, connection, protocol.ServerPong)
	if pong.Envelope.Type != protocol.ServerPong {
		t.Fatalf("server message = %q, want pong", pong.Envelope.Type)
	}
	if err := connection.Close(websocket.StatusNormalClosure, "test complete"); err != nil {
		t.Fatal(err)
	}
}

func readServerEnvelopeType(t *testing.T, connection *websocket.Conn, wanted protocol.ServerType) protocol.DecodedServer {
	t.Helper()
	for {
		message := readServerEnvelope(t, connection)
		if message.Envelope.Type == wanted {
			return message
		}
	}
}

func startHTTPTestSession(t *testing.T) (*Service, CreateSessionResponse, *httptest.Server) {
	t.Helper()
	service := NewService(ServiceConfig{AllowedOrigins: []string{"http://127.0.0.1"}, OnWebSocketError: func(err error) { t.Logf("WebSocket server error: %v", err) }})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	launchToken, err := service.NewLaunchToken()
	if err != nil {
		t.Fatal(err)
	}
	testServer := httptest.NewServer(service.Handler())
	t.Cleanup(testServer.Close)
	body, err := json.Marshal(map[string]any{"snapshot": testSnapshot(t, 2)})
	if err != nil {
		t.Fatal(err)
	}
	response := postJSON(t, testServer.URL+"/v1/sessions", launchToken, body)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d", response.StatusCode)
	}
	var created CreateSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	return service, created, testServer
}

func writeClientEnvelope(t *testing.T, connection *websocket.Conn, envelope protocol.ClientEnvelope, payload any) {
	t.Helper()
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Payload = payloadJSON
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := connection.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

func readServerEnvelope(t *testing.T, connection *websocket.Conn) protocol.DecodedServer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	messageType, data, err := connection.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("message type = %d, want text", messageType)
	}
	decoded, err := protocol.DecodeServer(data)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
