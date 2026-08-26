package replica

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/store"
	sqlitestore "sdmm/internal/aphelion/collab/store/sqlite"
)

func TestReplicaCommitsAcceptedOperationBeforeAcknowledging(t *testing.T) {
	session, value, local, fixture := newReplicaFixture(t)
	latestHash, err := fixture.FirstSnapshot.Hash()
	require.NoError(t, err)

	acknowledgement, err := session.ApplyAccepted(context.Background(), local.OwnerPublicKey, protocolv2.OperationAccepted{Operation: fixture.First, MapHash: latestHash})
	require.NoError(t, err)
	require.Equal(t, fixture.First.Revision, acknowledgement.Revision)
	require.Equal(t, latestHash, acknowledgement.MapHash)
	_, found, err := value.LookupOperation(context.Background(), local.DocumentID, fixture.First.OperationID)
	require.NoError(t, err)
	require.True(t, found)
	loaded, err := value.LoadClientSession(context.Background(), local.SessionID)
	require.NoError(t, err)
	require.Equal(t, fixture.First.Revision, loaded.AcknowledgedRevision)
}

func TestReplicaRejectsUnexpectedOwnerAndEntersDesynchronizedOnGap(t *testing.T) {
	session, value, local, fixture := newReplicaFixture(t)
	latestHash, err := fixture.Latest.Hash()
	require.NoError(t, err)

	_, err = session.ApplyAccepted(context.Background(), protocolv2.ActorKey{9}, protocolv2.OperationAccepted{Operation: fixture.First, MapHash: latestHash})
	require.ErrorContains(t, err, "owner")
	_, err = session.ApplyAccepted(context.Background(), local.OwnerPublicKey, protocolv2.OperationAccepted{Operation: fixture.Second, MapHash: latestHash})
	require.Error(t, err)
	require.Equal(t, StateDesynchronized, session.State())
	_, found, err := value.LookupOperation(context.Background(), local.DocumentID, fixture.Second.OperationID)
	require.NoError(t, err)
	require.False(t, found)
}

func TestReplicaInstallsSnapshotAndReturnsCaughtUp(t *testing.T) {
	session, value, local, fixture := newReplicaFixture(t)
	latestHash, err := fixture.Latest.Hash()
	require.NoError(t, err)
	session.MarkDesynchronized()

	complete, err := session.InstallSnapshot(context.Background(), local.OwnerPublicKey, protocolv2.SyncSnapshot{Snapshot: fixture.Latest, MapHash: latestHash})
	require.NoError(t, err)
	require.Equal(t, fixture.Latest.Revision, complete.Revision)
	require.Equal(t, StateCaughtUp, session.State())
	loaded, replay, err := value.Load(context.Background(), local.DocumentID)
	require.NoError(t, err)
	require.Equal(t, fixture.Latest.Revision, loaded.Revision)
	require.Empty(t, replay)
}

func newReplicaFixture(t *testing.T) (*Session, *sqlitestore.Store, store.LocalSession, store.ConformanceFixture) {
	t.Helper()
	fixture, err := store.NewConformanceFixture()
	require.NoError(t, err)
	value, err := sqlitestore.Open(filepath.Join(t.TempDir(), "replica.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = value.Close() })
	require.NoError(t, value.Create(context.Background(), fixture.Initial))
	manifest := []byte(`{"generation":1}`)
	initialHash, err := fixture.Initial.Hash()
	require.NoError(t, err)
	local := store.LocalSession{
		SessionID: "01900000-0000-7000-8000-000000000002", RelayURL: "https://mapping.a13.info",
		Role: protocolv2.RoleEditor, DocumentID: fixture.Initial.DocumentID, OwnerPublicKey: protocolv2.ActorKey{1},
		Manifest: manifest, ManifestSHA256: sha256.Sum256(manifest), AcknowledgedRevision: fixture.Initial.Revision,
		AcknowledgedMapHash: initialHash, DisplayName: "Test Editor", IdentitySecretRef: "identity/0123456789abcdef0123456789abcdef",
		GroupKeySecretRef: "group/0123456789abcdef0123456789abcdef",
	}
	require.NoError(t, value.CreateClientSession(context.Background(), local))
	session, err := NewSession(value, local)
	require.NoError(t, err)
	return session, value, local, fixture
}
