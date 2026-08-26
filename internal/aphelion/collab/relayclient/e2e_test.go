package relayclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/authority"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/relay"
	"sdmm/internal/aphelion/collab/replica"
	"sdmm/internal/aphelion/collab/store"
	sqlitestore "sdmm/internal/aphelion/collab/store/sqlite"
)

func TestOwnerAndReplicaConvergeThroughRealRelay(t *testing.T) {
	fixture, err := store.NewConformanceFixture()
	require.NoError(t, err)
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
	ownerID, err := model.NewActorID()
	require.NoError(t, err)
	roomID := protocolv2.RoomID{1}
	manifest := protocolv2.RoleManifest{RoomID: roomID, Generation: 1, OwnerKey: ownerKey, GroupKeyGeneration: 1, Members: []protocolv2.Member{
		{ActorKey: ownerKey, ActorID: ownerID, Role: protocolv2.RoleOwner, DisplayName: "Test Owner"},
	}}
	signedManifest, err := authority.SignManifest(manifest, ownerPrivate)
	require.NoError(t, err)
	ownerStore := store.NewMemoryStore()
	document, err := authority.StartDocument(context.Background(), fixture.Initial, ownerStore)
	require.NoError(t, err)
	t.Cleanup(func() { _ = document.Close(context.Background()) })
	ownerAuthority, err := authority.NewOwnerSession(document, ownerStore, ownerPrivate, signedManifest)
	require.NoError(t, err)
	groupKey := protocolv2.GroupKey{9}
	manifestDigest, err := authority.DigestManifest(manifest)
	require.NoError(t, err)
	now := time.Now().UTC()
	unsignedInvitation := protocolv2.Invitation{
		Version: protocolv2.Version, RelayURL: server.URL, RoomID: roomID, Role: protocolv2.RoleEditor,
		Admission: protocolv2.Capability{7}, GroupKey: groupKey, OwnerPublicKey: ownerKey, ManifestSHA256: manifestDigest,
		DocumentID: fixture.Initial.DocumentID, EnvironmentHash: fixture.Initial.EnvironmentHash,
		IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute),
	}
	encodedInvitation, err := protocolv2.EncodeInvitation(unsignedInvitation, ownerPrivate)
	require.NoError(t, err)
	invitation, err := protocolv2.ParseInvitation(encodedInvitation, now)
	require.NoError(t, err)
	require.NoError(t, ownerAuthority.RegisterInvitation(invitation, now))
	ownerTransport, err := ConnectOwner(context.Background(), server.URL, roomID, ownerPrivate)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ownerTransport.Close() })
	capability := [32]byte(invitation.Admission)
	require.NoError(t, ownerTransport.ReplaceAdmissions(context.Background(), 1, ownerAuthority.RelayAdmissions()))
	ownerContext, cancelOwner := context.WithCancel(context.Background())
	t.Cleanup(cancelOwner)
	ownerErrors := make(chan error, 1)
	go func() {
		ownerErrors <- (&Owner{Transport: ownerTransport, Authority: ownerAuthority, GroupKey: groupKey}).Serve(ownerContext)
	}()

	editorTransport, err := ConnectParticipant(context.Background(), server.URL, roomID, editorPrivate, capability)
	require.NoError(t, err)
	t.Cleanup(func() { _ = editorTransport.Close() })
	replicaStore, err := sqlitestore.Open(filepath.Join(t.TempDir(), "editor.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = replicaStore.Close() })
	require.NoError(t, replicaStore.Create(context.Background(), fixture.Initial))
	manifestBytes := []byte(`{"generation":1}`)
	initialHash, err := fixture.Initial.Hash()
	require.NoError(t, err)
	local := store.LocalSession{SessionID: "01900000-0000-7000-8000-000000000003", RelayURL: server.URL, Role: protocolv2.RoleEditor, DocumentID: fixture.Initial.DocumentID, OwnerPublicKey: ownerKey, Manifest: manifestBytes, ManifestSHA256: sha256.Sum256(manifestBytes), AcknowledgedRevision: fixture.Initial.Revision, AcknowledgedMapHash: initialHash, DisplayName: "Test Editor", IdentitySecretRef: "identity/0123456789abcdef0123456789abcdef", GroupKeySecretRef: "group/0123456789abcdef0123456789abcdef"}
	require.NoError(t, replicaStore.CreateClientSession(context.Background(), local))
	replicaSession, err := replica.NewSession(replicaStore, local)
	require.NoError(t, err)
	participant := &Participant{Transport: editorTransport, Replica: replicaSession, GroupKey: groupKey, ActorID: fixture.First.ActorID, DisplayName: "Test Editor", Invitation: &invitation}

	complete, err := participant.Synchronize(context.Background(), fixture.Initial.EnvironmentHash, nil)
	require.NoError(t, err)
	require.Equal(t, fixture.Initial.Revision, complete.Revision)
	participantExecutor, err := NewParticipantExecutor(participant, fixture.First.ActorID)
	require.NoError(t, err)
	executorContext, cancelExecutor := context.WithCancel(context.Background())
	t.Cleanup(cancelExecutor)
	participantExecutor.Start(executorContext)
	accepted, err := participantExecutor.Execute(context.Background(), fixture.First.Operation)
	require.NoError(t, err)
	require.Equal(t, fixture.First.OperationID, accepted.OperationID)
	ownerExecutor, err := NewOwnerExecutor(ownerTransport, ownerAuthority, groupKey)
	require.NoError(t, err)
	ownerOperation := fixture.Second.Operation
	ownerOperation.ActorID = ownerID
	ownerAccepted, err := ownerExecutor.Execute(context.Background(), ownerOperation)
	require.NoError(t, err)
	require.Equal(t, fixture.Second.Revision, ownerAccepted.Revision)
	require.Eventually(t, func() bool {
		snapshot, snapshotErr := participantExecutor.Snapshot(context.Background())
		return snapshotErr == nil && snapshot.Revision == ownerAccepted.Revision
	}, time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool {
		select {
		case err := <-ownerErrors:
			t.Fatalf("owner adapter stopped: %v", err)
		default:
		}
		snapshot, err := document.Snapshot(context.Background())
		return err == nil && snapshot.Revision == ownerAccepted.Revision
	}, time.Second, 5*time.Millisecond)
}
