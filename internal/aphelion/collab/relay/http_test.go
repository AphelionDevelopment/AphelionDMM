package relay

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/protocolv2"
)

func TestRelayHealthAndOpaqueOwnerRouting(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, validConfigYAML))
	require.NoError(t, err)
	service := NewService(config)
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)

	response, err := server.Client().Get(server.URL + "/v1/health/ready")
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	require.NoError(t, response.Body.Close())
	versionResponse, err := server.Client().Get(server.URL + "/v1/version")
	require.NoError(t, err)
	require.Equal(t, 200, versionResponse.StatusCode)
	require.NoError(t, versionResponse.Body.Close())
	metricsResponse, err := server.Client().Get(server.URL + "/metrics")
	require.NoError(t, err)
	require.Equal(t, 200, metricsResponse.StatusCode)
	require.NoError(t, metricsResponse.Body.Close())
	ownerPublic, ownerPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	editorPublic, editorPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var ownerKey, editorKey protocolv2.ActorKey
	copy(ownerKey[:], ownerPublic)
	copy(editorKey[:], editorPublic)
	roomID := protocolv2.RoomID{1}
	ownerConnection := dialRelay(t, server.URL)
	writeControl(t, ownerConnection, ownerPrivate, protocolv2.Header{Version: 2, Route: protocolv2.RouteControl, Kind: protocolv2.KindRoomCreate, RoomID: roomID, MessageID: protocolv2.MessageID{1}, Sender: ownerKey, Sequence: 1}, protocolv2.RoomCreateControl{})
	readRelayFrame(t, ownerConnection)
	capability := [32]byte{7}
	writeControl(t, ownerConnection, ownerPrivate, protocolv2.Header{Version: 2, Route: protocolv2.RouteControl, Kind: protocolv2.KindAdmissionReplace, RoomID: roomID, MessageID: protocolv2.MessageID{2}, Sender: ownerKey, Sequence: 2}, protocolv2.AdmissionReplaceControl{Generation: 1, Admissions: []protocolv2.AdmissionControl{{Digest: sha256.Sum256(capability[:]), Role: protocolv2.RoleEditor, ExpiresAt: time.Now().Add(time.Minute)}}})
	require.Eventually(t, func() bool {
		room, found := service.registry.Snapshot(roomID)
		return found && room.AdmissionGeneration == 1
	}, time.Second, 5*time.Millisecond)
	editorConnection := dialRelay(t, server.URL)
	writeControl(t, editorConnection, editorPrivate, protocolv2.Header{Version: 2, Route: protocolv2.RouteControl, Kind: protocolv2.KindConnect, RoomID: roomID, MessageID: protocolv2.MessageID{3}, Sender: editorKey, Sequence: 1}, protocolv2.ConnectControl{Admission: capability})
	readRelayFrame(t, editorConnection)

	groupKey := protocolv2.GroupKey{9}
	payload := []byte("relay-must-not-interpret-this-fixture")
	envelope, err := protocolv2.Seal(bytes.NewReader(bytes.Repeat([]byte{5}, 24)), groupKey, editorPrivate, protocolv2.Header{Version: 2, Route: protocolv2.RouteOwner, Kind: protocolv2.KindApplication, RoomID: roomID, MessageID: protocolv2.MessageID{4}, Sender: editorKey, Sequence: 2}, payload)
	require.NoError(t, err)
	encoded, err := protocolv2.MarshalEnvelope(envelope)
	require.NoError(t, err)
	require.NoError(t, editorConnection.Write(context.Background(), websocket.MessageBinary, encoded))
	messageType, routed, err := ownerConnection.Read(context.Background())
	require.NoError(t, err)
	require.Equal(t, websocket.MessageBinary, messageType)
	require.Equal(t, encoded, routed)
}

func TestRelayRequiresDualSignaturesForOwnerTransfer(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	service := NewService(Config{RoomIdleTTL: Duration(time.Minute), Limits: Limits{MaxConnections: 10, MaxRooms: 10, MaxConnectionsPerRoom: 10}})
	ownerPublic, ownerPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	editorPublic, editorPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var roomID protocolv2.RoomID
	roomID[0] = 1
	var ownerKey, editorKey protocolv2.ActorKey
	copy(ownerKey[:], ownerPublic)
	copy(editorKey[:], editorPublic)
	require.NoError(t, service.registry.CreateRoom(roomID, ownerKey, now))
	capability := []byte("transfer-capability")
	require.NoError(t, service.registry.ReplaceAdmissions(roomID, ownerKey, 1, []Admission{{Digest: sha256.Sum256(capability), Role: protocolv2.RoleEditor, ExpiresAt: now.Add(time.Minute)}}))
	_, err = service.registry.Connect(roomID, editorKey, capability, now)
	require.NoError(t, err)
	manifest := protocolv2.RoleManifest{RoomID: roomID, Generation: 2, OwnerKey: editorKey, Members: []protocolv2.Member{
		{ActorKey: ownerKey, Role: protocolv2.RoleEditor},
		{ActorKey: editorKey, Role: protocolv2.RoleOwner},
	}}
	offer := protocolv2.OwnershipOffer{TargetActor: editorKey, NewOwnerKey: editorKey, Generation: manifest.Generation, Manifest: manifest}
	digest, err := protocolv2.DigestOwnershipOffer(offer)
	require.NoError(t, err)
	transfer := protocolv2.OwnershipTransferred{Manifest: manifest, OfferSHA256: digest}
	copy(transfer.OldOwnerSignature[:], ed25519.Sign(ownerPrivate, digest[:]))
	copy(transfer.NewOwnerSignature[:], ed25519.Sign(editorPrivate, digest[:]))

	tampered := transfer
	tampered.Manifest.Members = append([]protocolv2.Member(nil), transfer.Manifest.Members...)
	tampered.Manifest.Members[0].DisplayName = "Tampered"
	require.ErrorContains(t, service.acceptOwnerTransfer(roomID, ownerKey, tampered, now), "digest")
	require.NoError(t, service.acceptOwnerTransfer(roomID, ownerKey, transfer, now))
	room, found := service.registry.Snapshot(roomID)
	require.True(t, found)
	require.Equal(t, editorKey, room.Owner)
}

func TestRelayUsesForwardedAddressOnlyFromTrustedProxy(t *testing.T) {
	service := NewService(Config{TrustedProxyCIDRs: []string{"10.0.0.0/8"}})
	request := httptest.NewRequest(http.MethodGet, "/v2/collaboration", nil)
	request.RemoteAddr = "10.0.0.2:1234"
	request.Header.Set("CF-Connecting-IP", "203.0.113.8")
	address, err := service.clientAddress(request)
	require.NoError(t, err)
	require.Equal(t, "203.0.113.8", address)

	request.RemoteAddr = "192.0.2.4:1234"
	address, err = service.clientAddress(request)
	require.NoError(t, err)
	require.Equal(t, "192.0.2.4", address)
}

func readRelayFrame(t *testing.T, connection *websocket.Conn) []byte {
	t.Helper()
	messageType, encoded, err := connection.Read(context.Background())
	require.NoError(t, err)
	require.Equal(t, websocket.MessageBinary, messageType)
	return encoded
}

func dialRelay(t *testing.T, baseURL string) *websocket.Conn {
	t.Helper()
	connection, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(baseURL, "http")+"/v2/collaboration", &websocket.DialOptions{Subprotocols: []string{WebSocketSubprotocol}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.CloseNow() })
	return connection
}

func writeControl(t *testing.T, connection *websocket.Conn, privateKey ed25519.PrivateKey, header protocolv2.Header, payload any) {
	t.Helper()
	envelope, err := protocolv2.SignControl(nil, privateKey, header, payload)
	require.NoError(t, err)
	encoded, err := protocolv2.MarshalEnvelope(envelope)
	require.NoError(t, err)
	require.NoError(t, connection.Write(context.Background(), websocket.MessageBinary, encoded))
}
