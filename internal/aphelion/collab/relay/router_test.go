package relay

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/protocolv2"
)

func TestRouterEnforcesOwnerAndActorRoutes(t *testing.T) {
	registry, roomID, owner, editor := routedRoom(t)
	router := NewRouter(registry)
	envelope := routedEnvelope(roomID, editor, protocolv2.RouteOwner)

	targets, err := router.Targets(envelope)
	require.NoError(t, err)
	require.Equal(t, []protocolv2.ActorKey{owner}, targets)
	envelope.Header.Route = protocolv2.RouteActor
	envelope.Header.Sender = owner
	envelope.Header.Recipient = editor
	targets, err = router.Targets(envelope)
	require.NoError(t, err)
	require.Equal(t, []protocolv2.ActorKey{editor}, targets)
	envelope.Header.Sender = editor
	_, err = router.Targets(envelope)
	require.ErrorContains(t, err, "owner")
}

func TestRouterRoomBroadcastIsOwnerOnlyAndPresenceIsMemberScoped(t *testing.T) {
	registry, roomID, owner, editor := routedRoom(t)
	viewerCapability := []byte("viewer-capability")
	viewer := protocolv2.ActorKey{4}
	require.NoError(t, registry.ReplaceAdmissions(roomID, owner, 2, []Admission{
		{Digest: sha256.Sum256([]byte("editor-capability")), Role: protocolv2.RoleEditor, ExpiresAt: time.Unix(200, 0).UTC(), BoundActor: editor},
		{Digest: sha256.Sum256(viewerCapability), Role: protocolv2.RoleViewer, ExpiresAt: time.Unix(200, 0).UTC()},
	}))
	_, err := registry.Connect(roomID, viewer, viewerCapability, time.Unix(101, 0).UTC())
	require.NoError(t, err)
	router := NewRouter(registry)

	targets, err := router.Targets(routedEnvelope(roomID, owner, protocolv2.RouteRoom))
	require.NoError(t, err)
	require.ElementsMatch(t, []protocolv2.ActorKey{editor, viewer}, targets)
	_, err = router.Targets(routedEnvelope(roomID, editor, protocolv2.RouteRoom))
	require.ErrorContains(t, err, "owner")
	targets, err = router.Targets(routedEnvelope(roomID, viewer, protocolv2.RoutePresence))
	require.NoError(t, err)
	require.ElementsMatch(t, []protocolv2.ActorKey{owner, editor}, targets)
}

func TestRouterReportsOwnerOfflineAndDisconnectedSender(t *testing.T) {
	registry, roomID, owner, editor := routedRoom(t)
	router := NewRouter(registry)
	registry.Disconnect(roomID, owner, time.Unix(102, 0).UTC())
	_, err := router.Targets(routedEnvelope(roomID, editor, protocolv2.RouteOwner))
	require.ErrorContains(t, err, "owner_offline")
	registry.Disconnect(roomID, editor, time.Unix(103, 0).UTC())
	_, err = router.Targets(routedEnvelope(roomID, editor, protocolv2.RoutePresence))
	require.ErrorContains(t, err, "not connected")
}

func routedRoom(t *testing.T) (*Registry, protocolv2.RoomID, protocolv2.ActorKey, protocolv2.ActorKey) {
	t.Helper()
	now := time.Unix(100, 0).UTC()
	registry := NewRegistry(time.Minute)
	roomID := protocolv2.RoomID{1}
	owner := protocolv2.ActorKey{2}
	editor := protocolv2.ActorKey{3}
	capability := []byte("editor-capability")
	require.NoError(t, registry.CreateRoom(roomID, owner, now))
	require.NoError(t, registry.ReplaceAdmissions(roomID, owner, 1, []Admission{{Digest: sha256.Sum256(capability), Role: protocolv2.RoleEditor, ExpiresAt: time.Unix(200, 0).UTC()}}))
	_, err := registry.Connect(roomID, editor, capability, now)
	require.NoError(t, err)
	return registry, roomID, owner, editor
}

func routedEnvelope(roomID protocolv2.RoomID, sender protocolv2.ActorKey, route protocolv2.Route) protocolv2.Envelope {
	return protocolv2.Envelope{Header: protocolv2.Header{
		Version: protocolv2.Version, Route: route, Kind: protocolv2.KindApplication, RoomID: roomID,
		MessageID: protocolv2.MessageID{1}, Sender: sender, Sequence: 1,
	}, Nonce: protocolv2.Nonce{1}, Ciphertext: []byte{1}, Signature: protocolv2.Signature{1}}
}
