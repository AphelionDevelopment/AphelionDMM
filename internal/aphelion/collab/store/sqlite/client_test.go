package sqlite

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
	collabstore "sdmm/internal/aphelion/collab/store"
)

func TestClientSessionRoundTripKeepsOnlySecretReferences(t *testing.T) {
	fixture := mustConformanceFixture(t)
	path := filepath.Join(t.TempDir(), "client.db")
	value, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = value.Close() })
	require.NoError(t, value.Create(context.Background(), fixture.Initial))
	session := testLocalSession(t, fixture.Initial)

	require.NoError(t, value.CreateClientSession(context.Background(), session))
	loaded, err := value.LoadClientSession(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Equal(t, session, loaded)

	var raw string
	require.NoError(t, value.database.QueryRow("SELECT identity_secret_ref || group_key_secret_ref || owner_secret_ref FROM client_sessions WHERE session_id = ?", session.SessionID).Scan(&raw))
	require.NotContains(t, raw, "private-key-material")
	require.NotContains(t, raw, "group-key-material")
}

func TestClientPendingSurvivesReopenAndResolvesExactlyOne(t *testing.T) {
	fixture := mustConformanceFixture(t)
	path := filepath.Join(t.TempDir(), "pending.db")
	value, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, value.Create(context.Background(), fixture.Initial))
	session := testLocalSession(t, fixture.Initial)
	require.NoError(t, value.CreateClientSession(context.Background(), session))
	pending := collabstore.PendingSubmission{SessionID: session.SessionID, Operation: fixture.First.Operation, Disposition: collabstore.PendingReady, CreatedAt: time.Unix(10, 0).UTC()}
	require.NoError(t, value.SavePending(context.Background(), pending))
	require.NoError(t, value.Close())

	reopened, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	items, err := reopened.ListPending(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Equal(t, []collabstore.PendingSubmission{pending}, items)
	require.NoError(t, reopened.ResolvePending(context.Background(), session.SessionID, pending.Operation.OperationID, collabstore.PendingConflicting, "base changed"))
	items, err = reopened.ListPending(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Equal(t, collabstore.PendingConflicting, items[0].Disposition)
	require.Equal(t, "base changed", items[0].Diagnostic)
	require.Error(t, reopened.ResolvePending(context.Background(), session.SessionID, model.OperationID("01900000-0000-7000-8000-000000000099"), collabstore.PendingObsolete, "missing"))
}

func TestClientManifestAndAdmissionsReplaceAtomically(t *testing.T) {
	fixture := mustConformanceFixture(t)
	value, err := Open(filepath.Join(t.TempDir(), "manifest.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = value.Close() })
	require.NoError(t, value.Create(context.Background(), fixture.Initial))
	session := testLocalSession(t, fixture.Initial)
	require.NoError(t, value.CreateClientSession(context.Background(), session))
	manifest := []byte(`{"generation":2}`)
	admission := collabstore.Admission{SessionID: session.SessionID, CapabilityID: [32]byte{1}, Role: protocolv2.RoleEditor, ExpiresAt: time.Unix(100, 0).UTC(), BoundActor: protocolv2.ActorKey{2}}

	require.NoError(t, value.SaveManifest(context.Background(), session.SessionID, manifest, []collabstore.Admission{admission}))
	admissions, err := value.ListAdmissions(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Equal(t, []collabstore.Admission{admission}, admissions)
	loaded, err := value.LoadClientSession(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Equal(t, manifest, loaded.Manifest)
	require.Equal(t, sha256.Sum256(manifest), loaded.ManifestSHA256)
}

func TestClientManifestReplacementRollsBackOnAdmissionFailure(t *testing.T) {
	fixture := mustConformanceFixture(t)
	value, err := Open(filepath.Join(t.TempDir(), "manifest-rollback.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = value.Close() })
	require.NoError(t, value.Create(context.Background(), fixture.Initial))
	session := testLocalSession(t, fixture.Initial)
	require.NoError(t, value.CreateClientSession(context.Background(), session))
	require.NoError(t, value.database.QueryRow("SELECT manifest FROM client_sessions WHERE session_id = ?", session.SessionID).Scan(&session.Manifest))
	require.NoError(t, func() error {
		_, err := value.database.Exec(`CREATE TRIGGER fail_admission BEFORE INSERT ON admissions BEGIN SELECT RAISE(ABORT, 'injected admission failure'); END`)
		return err
	}())
	admission := collabstore.Admission{SessionID: session.SessionID, CapabilityID: [32]byte{1}, Role: protocolv2.RoleEditor, ExpiresAt: time.Unix(100, 0).UTC()}

	require.Error(t, value.SaveManifest(context.Background(), session.SessionID, []byte(`{"generation":2}`), []collabstore.Admission{admission}))
	loaded, err := value.LoadClientSession(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Equal(t, session.Manifest, loaded.Manifest)
	admissions, err := value.ListAdmissions(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Empty(t, admissions)
}

func TestLoadClientSessionRejectsCorruptManifest(t *testing.T) {
	fixture := mustConformanceFixture(t)
	value, err := Open(filepath.Join(t.TempDir(), "corrupt-manifest.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = value.Close() })
	require.NoError(t, value.Create(context.Background(), fixture.Initial))
	session := testLocalSession(t, fixture.Initial)
	require.NoError(t, value.CreateClientSession(context.Background(), session))
	_, err = value.database.Exec("UPDATE client_sessions SET manifest = ? WHERE session_id = ?", []byte(`{"tampered":true}`), session.SessionID)
	require.NoError(t, err)

	_, err = value.LoadClientSession(context.Background(), session.SessionID)
	require.ErrorContains(t, err, "manifest digest is invalid")
}

func TestReplaceReplicaVerifiesThenReplacesAtomically(t *testing.T) {
	fixture := mustConformanceFixture(t)
	value, err := Open(filepath.Join(t.TempDir(), "replace.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = value.Close() })
	require.NoError(t, value.Create(context.Background(), fixture.Initial))
	session := testLocalSession(t, fixture.Initial)
	require.NoError(t, value.CreateClientSession(context.Background(), session))
	latestHash, err := fixture.Latest.Hash()
	require.NoError(t, err)
	session.AcknowledgedRevision = fixture.Latest.Revision
	session.AcknowledgedMapHash = latestHash

	require.NoError(t, value.ReplaceReplica(context.Background(), session, fixture.Initial, []model.AcceptedOperation{fixture.First, fixture.Second}))
	_, replay, err := value.Load(context.Background(), fixture.Initial.DocumentID)
	require.NoError(t, err)
	require.Equal(t, []model.AcceptedOperation{fixture.First, fixture.Second}, replay)
	loaded, err := value.LoadClientSession(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Equal(t, fixture.Latest.Revision, loaded.AcknowledgedRevision)
	require.Equal(t, latestHash, loaded.AcknowledgedMapHash)

	bad := fixture.Second
	bad.Revision++
	require.Error(t, value.ReplaceReplica(context.Background(), session, fixture.Initial, []model.AcceptedOperation{fixture.First, bad}))
	_, replay, err = value.Load(context.Background(), fixture.Initial.DocumentID)
	require.NoError(t, err)
	require.Equal(t, []model.AcceptedOperation{fixture.First, fixture.Second}, replay)
}

func TestOpenMigratesClientSchemaFromVersionTwo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema-two.db")
	value, err := Open(path)
	require.NoError(t, err)
	for _, table := range []string{"pending_submissions", "actor_profiles", "admissions", "client_sessions"} {
		_, err := value.database.Exec("DROP TABLE " + table)
		require.NoError(t, err)
	}
	_, err = value.database.Exec("PRAGMA user_version = 2")
	require.NoError(t, err)
	require.NoError(t, value.Close())

	reopened, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	var version int
	require.NoError(t, reopened.database.QueryRow("PRAGMA user_version").Scan(&version))
	require.Equal(t, 3, version)
}

func mustConformanceFixture(t *testing.T) collabstore.ConformanceFixture {
	t.Helper()
	fixture, err := collabstore.NewConformanceFixture()
	require.NoError(t, err)
	return fixture
}

func testLocalSession(t *testing.T, snapshot model.Snapshot) collabstore.LocalSession {
	t.Helper()
	manifest := []byte(`{"generation":1}`)
	mapHash, err := snapshot.Hash()
	require.NoError(t, err)
	return collabstore.LocalSession{
		SessionID:            "01900000-0000-7000-8000-000000000001",
		RelayURL:             "https://mapping.a13.info",
		Role:                 protocolv2.RoleOwner,
		DocumentID:           snapshot.DocumentID,
		OwnerPublicKey:       protocolv2.ActorKey{1},
		Manifest:             manifest,
		ManifestSHA256:       sha256.Sum256(manifest),
		AcknowledgedRevision: snapshot.Revision,
		AcknowledgedMapHash:  mapHash,
		DisplayName:          "Test Owner",
		IdentitySecretRef:    "identity/0123456789abcdef0123456789abcdef",
		GroupKeySecretRef:    "group/0123456789abcdef0123456789abcdef",
		OwnerSecretRef:       "capability/0123456789abcdef0123456789abcdef",
	}
}
