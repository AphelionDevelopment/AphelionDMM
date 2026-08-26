package authority

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/store"
)

func TestOwnerSyncUsesReplayFromKnownRetainedRevision(t *testing.T) {
	session, editor, fixture := newTestOwnerSession(t, store.NewMemoryStore())
	operation := fixture.First.Operation
	operation.ActorID = editor.ActorID
	_, err := session.Handle(context.Background(), editor.ActorKey, protocolv2.DecodedApplication{Type: protocolv2.ApplicationOperationSubmit, Payload: &protocolv2.OperationSubmit{Operation: operation}})
	require.NoError(t, err)
	initialHash, err := fixture.Initial.Hash()
	require.NoError(t, err)

	response, err := session.Sync(context.Background(), editor.ActorKey, protocolv2.SyncHello{
		ActorKey: editor.ActorKey, Role: editor.Role, Revision: fixture.Initial.Revision,
		MapHash: initialHash, EnvironmentHash: fixture.Initial.EnvironmentHash,
	})
	require.NoError(t, err)
	require.Equal(t, protocolv2.ApplicationSyncReplay, response.Type)
	replay := response.Payload.(*protocolv2.SyncReplay)
	require.Len(t, replay.Operations, 1)
	require.Equal(t, operation.OperationID, replay.Operations[0].OperationID)
}

func TestOwnerSyncUsesSnapshotForUnknownHash(t *testing.T) {
	session, editor, fixture := newTestOwnerSession(t, store.NewMemoryStore())

	response, err := session.Sync(context.Background(), editor.ActorKey, protocolv2.SyncHello{
		ActorKey: editor.ActorKey, Role: editor.Role, Revision: fixture.Initial.Revision,
		MapHash:         "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		EnvironmentHash: fixture.Initial.EnvironmentHash,
	})
	require.NoError(t, err)
	require.Equal(t, protocolv2.ApplicationSyncSnapshot, response.Type)
	snapshot := response.Payload.(*protocolv2.SyncSnapshot)
	require.Equal(t, fixture.Initial.DocumentID, snapshot.Snapshot.DocumentID)
}

func TestOwnerSyncRejectsRoleAndEnvironmentMismatch(t *testing.T) {
	session, editor, fixture := newTestOwnerSession(t, store.NewMemoryStore())
	initialHash, err := fixture.Initial.Hash()
	require.NoError(t, err)
	hello := protocolv2.SyncHello{ActorKey: editor.ActorKey, Role: protocolv2.RoleViewer, Revision: fixture.Initial.Revision, MapHash: initialHash, EnvironmentHash: fixture.Initial.EnvironmentHash}

	_, err = session.Sync(context.Background(), editor.ActorKey, hello)
	require.ErrorContains(t, err, "role")
	hello.Role = editor.Role
	hello.EnvironmentHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	_, err = session.Sync(context.Background(), editor.ActorKey, hello)
	require.ErrorContains(t, err, "environment")
}

func TestOwnerSyncAdmitsFirstActorFromRegisteredSignedInvitation(t *testing.T) {
	session, _, fixture := newTestOwnerSession(t, store.NewMemoryStore())
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var actorKey protocolv2.ActorKey
	copy(actorKey[:], publicKey)
	actorID, err := model.NewActorID()
	require.NoError(t, err)
	manifestDigest, err := DigestManifest(session.Manifest())
	require.NoError(t, err)
	now := time.Now().UTC()
	unsigned := protocolv2.Invitation{
		Version: protocolv2.Version, RelayURL: "https://mapping.a13.info", RoomID: session.Manifest().RoomID,
		Role: protocolv2.RoleEditor, Admission: protocolv2.Capability{7}, GroupKey: protocolv2.GroupKey{8},
		DocumentID: fixture.Initial.DocumentID, EnvironmentHash: fixture.Initial.EnvironmentHash,
		OwnerPublicKey: session.Manifest().OwnerKey, ManifestSHA256: manifestDigest, IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour),
	}
	encoded, err := protocolv2.EncodeInvitation(unsigned, session.signer)
	require.NoError(t, err)
	invitation, err := protocolv2.ParseInvitation(encoded, now)
	require.NoError(t, err)
	require.NoError(t, session.RegisterInvitation(invitation, now))
	initialHash, err := fixture.Initial.Hash()
	require.NoError(t, err)

	response, err := session.Sync(context.Background(), actorKey, protocolv2.SyncHello{
		ActorKey: actorKey, ActorID: actorID, DisplayName: "New Editor", Role: protocolv2.RoleEditor,
		Revision: fixture.Initial.Revision, MapHash: initialHash, EnvironmentHash: fixture.Initial.EnvironmentHash, Invitation: &invitation,
	})
	require.NoError(t, err)
	require.Equal(t, protocolv2.ApplicationSyncReplay, response.Type)
	require.Equal(t, actorKey, session.Manifest().Members[2].ActorKey)
	require.Equal(t, actorKey, session.RelayAdmissions()[0].BoundActor)
}
