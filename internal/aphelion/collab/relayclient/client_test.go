package relayclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/relay"
)

func TestOwnerAndParticipantRouteEncryptedApplicationThroughRealRelay(t *testing.T) {
	service := relay.NewService(loadRelayConfig(t))
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	ownerPublic, ownerPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	editorPublic, editorPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var ownerKey, editorKey protocolv2.ActorKey
	copy(ownerKey[:], ownerPublic)
	copy(editorKey[:], editorPublic)
	roomID := protocolv2.RoomID{1}
	owner, err := ConnectOwner(context.Background(), server.URL, roomID, ownerPrivate)
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close() })
	capability := [32]byte{7}
	require.NoError(t, owner.ReplaceAdmissions(context.Background(), 1, []protocolv2.AdmissionControl{{Digest: sha256.Sum256(capability[:]), Role: protocolv2.RoleEditor, ExpiresAt: time.Now().Add(time.Minute)}}))
	editor, err := ConnectParticipant(context.Background(), server.URL, roomID, editorPrivate, capability)
	require.NoError(t, err)
	t.Cleanup(func() { _ = editor.Close() })
	groupKey := protocolv2.GroupKey{9}
	expected := protocolv2.ProfileUpdate{DisplayName: "Test Editor", Sequence: 1}

	require.NoError(t, editor.SendApplication(context.Background(), groupKey, protocolv2.RouteOwner, protocolv2.ActorKey{}, protocolv2.ApplicationProfileUpdate, expected))
	sender, decoded, err := owner.ReadApplication(context.Background(), groupKey)
	require.NoError(t, err)
	require.Equal(t, editorKey, sender)
	require.Equal(t, &expected, decoded.Payload)
	require.Equal(t, ownerKey, owner.ActorKey())
}

func TestParticipantRetriesWhileOwnerAdmissionUpdateIsInFlight(t *testing.T) {
	service := relay.NewService(loadRelayConfig(t))
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	_, ownerPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, editorPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	roomID := protocolv2.RoomID{1}
	owner, err := ConnectOwner(context.Background(), server.URL, roomID, ownerPrivate)
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close() })
	capability := [32]byte{7}
	updated := make(chan error, 1)
	go func() {
		time.Sleep(40 * time.Millisecond)
		updated <- owner.ReplaceAdmissions(context.Background(), 1, []protocolv2.AdmissionControl{{Digest: sha256.Sum256(capability[:]), Role: protocolv2.RoleEditor, ExpiresAt: time.Now().Add(time.Minute)}})
	}()

	editor, err := ConnectParticipant(context.Background(), server.URL, roomID, editorPrivate, capability)
	require.NoError(t, err)
	t.Cleanup(func() { _ = editor.Close() })
	require.NoError(t, <-updated)
}

func TestClientRejectsCleartextNonLoopbackRelay(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, err = ConnectOwner(context.Background(), "http://example.invalid", protocolv2.RoomID{1}, privateKey)
	require.ErrorContains(t, err, "HTTPS")
}

func TestClientRejectsInboundReplayAndNonMonotonicSequence(t *testing.T) {
	client := &Client{}
	sender := protocolv2.ActorKey{1}
	first := protocolv2.Header{Version: protocolv2.Version, Route: protocolv2.RouteRoom, Kind: protocolv2.KindApplication, RoomID: protocolv2.RoomID{1}, MessageID: protocolv2.MessageID{1}, Sender: sender, Sequence: 1}
	require.NoError(t, client.acceptInbound(first))
	replayedID := first
	replayedID.Sequence = 2
	require.ErrorContains(t, client.acceptInbound(replayedID), "message ID")
	nonMonotonic := first
	nonMonotonic.MessageID = protocolv2.MessageID{2}
	require.Error(t, client.acceptInbound(nonMonotonic))
}

func TestParticipantExecutorRejectsViewerMutationBeforeSending(t *testing.T) {
	execution := &ParticipantExecutor{role: protocolv2.RoleViewer}
	_, err := execution.Execute(context.Background(), model.Operation{})
	require.ErrorContains(t, err, "read only")
}

func loadRelayConfig(t *testing.T) relay.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "relay.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`version: 1
public_origin: https://mapping.a13.info
bind_address: 127.0.0.1:0
trusted_proxy_cidrs:
  - 127.0.0.1/32
room_idle_ttl: 30m
limits:
  max_rooms: 10
  max_connections: 20
  max_connections_per_room: 8
  max_message_bytes: 1048576
  max_room_bytes_per_second: 8388608
  connect_burst: 30
  connect_window: 1m
  message_burst: 240
  message_window: 1s
observability:
  log_level: info
  metrics_bind_address: 127.0.0.1:0
`), 0o600))
	config, err := relay.LoadConfig(path)
	require.NoError(t, err)
	return config
}
