package relay

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/protocolv2"
)

func TestRegistryConsumesAdmissionByFirstActorAndAllowsBoundReconnect(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	registry := NewRegistry(time.Minute)
	roomID := protocolv2.RoomID{1}
	owner := protocolv2.ActorKey{2}
	editor := protocolv2.ActorKey{3}
	other := protocolv2.ActorKey{4}
	capability := []byte("one-use-test-capability")
	require.NoError(t, registry.CreateRoom(roomID, owner, now))
	require.NoError(t, registry.ReplaceAdmissions(roomID, owner, 1, []Admission{{Digest: sha256.Sum256(capability), Role: protocolv2.RoleEditor, ExpiresAt: now.Add(time.Minute)}}))

	role, err := registry.Connect(roomID, editor, capability, now)
	require.NoError(t, err)
	require.Equal(t, protocolv2.RoleEditor, role)
	registry.Disconnect(roomID, editor, now.Add(time.Second))
	role, err = registry.Connect(roomID, editor, capability, now.Add(2*time.Second))
	require.NoError(t, err)
	require.Equal(t, protocolv2.RoleEditor, role)
	_, err = registry.Connect(roomID, other, capability, now.Add(3*time.Second))
	require.ErrorContains(t, err, "bound")
}

func TestRegistryRejectsExpiredAdmissionAndWrongOwnerReplacement(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	registry := NewRegistry(time.Minute)
	roomID := protocolv2.RoomID{1}
	owner := protocolv2.ActorKey{2}
	capability := []byte("expired-test-capability")
	require.NoError(t, registry.CreateRoom(roomID, owner, now))
	require.Error(t, registry.ReplaceAdmissions(roomID, protocolv2.ActorKey{9}, 1, nil))
	require.NoError(t, registry.ReplaceAdmissions(roomID, owner, 1, []Admission{{Digest: sha256.Sum256(capability), Role: protocolv2.RoleViewer, ExpiresAt: now}}))
	_, err := registry.Connect(roomID, protocolv2.ActorKey{3}, capability, now.Add(time.Second))
	require.ErrorContains(t, err, "expired")
}

func TestRegistrySweepsOnlyOwnerOfflineExpiredRooms(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	registry := NewRegistry(time.Minute)
	roomID := protocolv2.RoomID{1}
	owner := protocolv2.ActorKey{2}
	require.NoError(t, registry.CreateRoom(roomID, owner, now))
	require.Empty(t, registry.Sweep(now.Add(2*time.Minute)))
	registry.Disconnect(roomID, owner, now.Add(2*time.Minute))
	require.Empty(t, registry.Sweep(now.Add(2*time.Minute+30*time.Second)))
	require.Equal(t, []protocolv2.RoomID{roomID}, registry.Sweep(now.Add(4*time.Minute)))
}

func TestRegistryEnforcesRoomAndPerRoomConnectionLimits(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	registry := NewRegistryWithLimits(time.Minute, 1, 2)
	roomID := protocolv2.RoomID{1}
	owner := protocolv2.ActorKey{2}
	require.NoError(t, registry.CreateRoom(roomID, owner, now))
	require.ErrorContains(t, registry.CreateRoom(protocolv2.RoomID{9}, protocolv2.ActorKey{8}, now), "room limit")
	firstCapability := []byte("first-capability")
	secondCapability := []byte("second-capability")
	require.NoError(t, registry.ReplaceAdmissions(roomID, owner, 1, []Admission{
		{Digest: sha256.Sum256(firstCapability), Role: protocolv2.RoleEditor, ExpiresAt: now.Add(time.Minute)},
		{Digest: sha256.Sum256(secondCapability), Role: protocolv2.RoleViewer, ExpiresAt: now.Add(time.Minute)},
	}))
	_, err := registry.Connect(roomID, protocolv2.ActorKey{3}, firstCapability, now)
	require.NoError(t, err)
	_, err = registry.Connect(roomID, protocolv2.ActorKey{4}, secondCapability, now)
	require.ErrorContains(t, err, "connection limit")
}

func TestRegistryTransfersOwnerOnlyWhileBothActorsAreConnected(t *testing.T) {
	registry, roomID, owner, editor := routedRoom(t)
	require.NoError(t, registry.TransferOwner(roomID, owner, editor, time.Unix(101, 0).UTC()))
	room, found := registry.Snapshot(roomID)
	require.True(t, found)
	require.Equal(t, editor, room.Owner)
	require.Equal(t, protocolv2.RoleEditor, room.Actors[owner])
	require.Equal(t, protocolv2.RoleOwner, room.Actors[editor])
	require.Error(t, registry.TransferOwner(roomID, owner, protocolv2.ActorKey{9}, time.Unix(102, 0).UTC()))
}
