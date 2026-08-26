package relayclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/relay"
)

func TestRelayRestartRestoresRoomOnlyFromOwnerClient(t *testing.T) {
	ownerPublic, ownerPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	editorPublic, editorPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var ownerKey, editorKey protocolv2.ActorKey
	copy(ownerKey[:], ownerPublic)
	copy(editorKey[:], editorPublic)
	roomID := protocolv2.RoomID{1}
	capability := [32]byte{7}
	admission := protocolv2.AdmissionControl{Digest: sha256.Sum256(capability[:]), Role: protocolv2.RoleEditor, ExpiresAt: time.Now().Add(time.Minute), BoundActor: editorKey}

	firstServer := httptest.NewServer(relay.NewService(loadRelayConfig(t)).Handler())
	owner, err := ConnectOwner(context.Background(), firstServer.URL, roomID, ownerPrivate)
	require.NoError(t, err)
	require.NoError(t, owner.ReplaceAdmissions(context.Background(), 1, []protocolv2.AdmissionControl{admission}))
	editor, err := ConnectParticipant(context.Background(), firstServer.URL, roomID, editorPrivate, capability)
	require.NoError(t, err)
	require.NoError(t, editor.Close())
	require.NoError(t, owner.Close())
	firstServer.Close()

	secondServer := httptest.NewServer(relay.NewService(loadRelayConfig(t)).Handler())
	t.Cleanup(secondServer.Close)
	owner, err = ConnectOwner(context.Background(), secondServer.URL, roomID, ownerPrivate)
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close() })
	require.NoError(t, owner.ReplaceAdmissions(context.Background(), 1, []protocolv2.AdmissionControl{admission}))
	editor, err = ConnectParticipant(context.Background(), secondServer.URL, roomID, editorPrivate, capability)
	require.NoError(t, err)
	t.Cleanup(func() { _ = editor.Close() })
	groupKey := protocolv2.GroupKey{9}
	require.NoError(t, editor.SendApplication(context.Background(), groupKey, protocolv2.RouteOwner, protocolv2.ActorKey{}, protocolv2.ApplicationProfileUpdate, protocolv2.ProfileUpdate{DisplayName: "Test Editor", Sequence: 1}))
	sender, message, err := owner.ReadApplication(context.Background(), groupKey)
	require.NoError(t, err)
	require.Equal(t, editorKey, sender)
	require.Equal(t, protocolv2.ApplicationProfileUpdate, message.Type)
}
