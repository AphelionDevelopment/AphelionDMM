package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/model"
	collabstore "sdmm/internal/aphelion/collab/store"
)

func TestRecoverDocumentReplaysAcknowledgedOperations(t *testing.T) {
	t.Parallel()

	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	value := collabstore.NewMemoryStore()
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	if err := value.Append(context.Background(), fixture.First); err != nil {
		t.Fatal(err)
	}
	if err := value.Append(context.Background(), fixture.Second); err != nil {
		t.Fatal(err)
	}
	owner, err := RecoverDocument(context.Background(), fixture.Initial.DocumentID, value)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	recovered, err := owner.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantHash, err := fixture.Latest.Hash()
	if err != nil {
		t.Fatal(err)
	}
	gotHash, err := recovered.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Revision != fixture.Latest.Revision || gotHash != wantHash {
		t.Fatalf("recovered revision/hash = %d/%s, want %d/%s", recovered.Revision, gotHash, fixture.Latest.Revision, wantHash)
	}
}

func TestRecoverDocumentReplaysAfterCompactedSnapshot(t *testing.T) {
	t.Parallel()

	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	value := collabstore.NewMemoryStore()
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	if err := value.Append(context.Background(), fixture.First); err != nil {
		t.Fatal(err)
	}
	if err := value.Append(context.Background(), fixture.Second); err != nil {
		t.Fatal(err)
	}
	if err := value.SaveSnapshot(context.Background(), fixture.FirstSnapshot); err != nil {
		t.Fatal(err)
	}
	owner, err := RecoverDocument(context.Background(), fixture.Initial.DocumentID, value)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	assertRecoveredSnapshot(t, owner, fixture.Latest)
}

func TestRecoverDocumentRejectsCorruptStoredSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	snapshot.MaxX = 0
	value := &recoveryStore{snapshot: snapshot}
	if _, err := RecoverDocument(context.Background(), snapshot.DocumentID, value); err == nil || !strings.Contains(err.Error(), "open stored snapshot") {
		t.Fatalf("RecoverDocument() error = %v, want corrupt snapshot error", err)
	}
}

func TestStartOrRecoverDocumentRejectsWrongBaselineHash(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	value := NewMemoryStore()
	owner, err := StartDocument(context.Background(), snapshot, value)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	mismatched := model.CloneSnapshot(snapshot)
	stableID, err := model.NewStableID()
	if err != nil {
		t.Fatal(err)
	}
	mismatched.Tiles = []model.Tile{{
		Coord: model.Coord{X: 1, Y: 1, Z: 1},
		State: model.TileState{Prefabs: []model.PrefabState{{StableID: stableID, Path: "/turf/open/floor", Vars: map[string]string{}}}},
	}}
	if _, err := startOrRecoverDocument(context.Background(), mismatched, value, DocumentConfig{}); err == nil || !strings.Contains(err.Error(), "baseline does not match") {
		t.Fatalf("startOrRecoverDocument() error = %v, want baseline mismatch", err)
	}
}

func TestInterruptedSnapshotDoesNotLoseAcknowledgedOperation(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	snapshotErrors := make(chan error, 1)
	value := &interruptedSnapshotStore{MemoryStore: NewMemoryStore()}
	owner, err := StartDocumentWithConfig(context.Background(), snapshot, value, DocumentConfig{
		SnapshotOperationThreshold: 1,
		OnSnapshotError: func(err error) {
			snapshotErrors <- err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := owner.Submit(context.Background(), testOperation(t, snapshot, 1))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-snapshotErrors:
		if !errors.Is(err, errSnapshotInterrupted) {
			t.Fatalf("snapshot error = %v, want %v", err, errSnapshotInterrupted)
		}
	case <-time.After(time.Second):
		t.Fatal("interrupted snapshot error was not reported")
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered, err := RecoverDocument(context.Background(), snapshot.DocumentID, value)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recovered.Close(context.Background()) })
	current, err := recovered.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != accepted.Revision {
		t.Fatalf("recovered revision = %d, want acknowledged revision %d", current.Revision, accepted.Revision)
	}
}

func TestRecoverDocumentRejectsDiscontinuousReplay(t *testing.T) {
	t.Parallel()

	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	value := &recoveryStore{snapshot: fixture.Initial, operations: []model.AcceptedOperation{fixture.Second}}
	if _, err := RecoverDocument(context.Background(), fixture.Initial.DocumentID, value); err == nil || !strings.Contains(err.Error(), "replay revision") {
		t.Fatalf("RecoverDocument() error = %v, want replay revision error", err)
	}
}

func TestStartOrRecoverDocumentRestoresExistingState(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	value := NewMemoryStore()
	owner, err := StartDocument(context.Background(), snapshot, value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Submit(context.Background(), testOperation(t, snapshot, 1)); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered, err := startOrRecoverDocument(context.Background(), snapshot, value, DocumentConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recovered.Close(context.Background()) })
	current, err := recovered.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 1 {
		t.Fatalf("recovered revision = %d, want 1", current.Revision)
	}
}

func TestNewServiceUsesConfiguredStore(t *testing.T) {
	t.Parallel()

	value := NewMemoryStore()
	service := NewService(ServiceConfig{Store: value, Document: DocumentConfig{SnapshotOperationThreshold: 7}})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	if service.store != value {
		t.Fatal("NewService() did not retain the configured store")
	}
	if service.documentConfig.SnapshotOperationThreshold != 7 {
		t.Fatalf("document snapshot threshold = %d, want 7", service.documentConfig.SnapshotOperationThreshold)
	}
}

func TestCreateSessionRecoversConfiguredStore(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	value := NewMemoryStore()
	owner, err := StartDocument(context.Background(), snapshot, value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Submit(context.Background(), testOperation(t, snapshot, 1)); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{Store: value})
	server := httptest.NewServer(service.Handler())
	t.Cleanup(func() {
		server.Close()
		_ = service.Shutdown(context.Background())
	})
	token, err := service.NewLaunchTokenForSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	created := createTestSession(t, server.URL, token, snapshot)
	if created.Revision != 1 {
		t.Fatalf("created revision = %d, want recovered revision 1", created.Revision)
	}
}

func TestReadinessTracksDocumentRecoveryWithoutFailingLiveness(t *testing.T) {
	t.Parallel()

	service := NewService(ServiceConfig{})
	server := httptest.NewServer(service.Handler())
	t.Cleanup(func() {
		server.Close()
		_ = service.Shutdown(context.Background())
	})
	documentID, err := model.NewDocumentID()
	if err != nil {
		t.Fatal(err)
	}
	service.setDocumentRecoveryError(documentID, errors.New("corrupt replay"))
	for path, want := range map[string]int{"/v1/health/live": http.StatusOK, "/v1/health/ready": http.StatusServiceUnavailable} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("GET %s status = %d, want %d", path, response.StatusCode, want)
		}
	}
	service.setDocumentRecoveryError(documentID, nil)
	response, err := http.Get(server.URL + "/v1/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("ready status after recovery = %d, want 200", response.StatusCode)
	}
}

type recoveryStore struct {
	snapshot   model.Snapshot
	operations []model.AcceptedOperation
}

func (store *recoveryStore) Create(context.Context, model.Snapshot) error          { return nil }
func (store *recoveryStore) Append(context.Context, model.AcceptedOperation) error { return nil }
func (store *recoveryStore) SaveSnapshot(context.Context, model.Snapshot) error    { return nil }
func (store *recoveryStore) RevisionHash(context.Context, model.DocumentID, model.Revision) (string, bool, error) {
	return "", false, nil
}
func (store *recoveryStore) LookupOperation(context.Context, model.DocumentID, model.OperationID) (model.AcceptedOperation, bool, error) {
	return model.AcceptedOperation{}, false, nil
}
func (store *recoveryStore) Load(context.Context, model.DocumentID) (model.Snapshot, []model.AcceptedOperation, error) {
	return model.CloneSnapshot(store.snapshot), append([]model.AcceptedOperation(nil), store.operations...), nil
}
func (store *recoveryStore) Close() error { return nil }

var errSnapshotInterrupted = errors.New("snapshot interrupted")

type interruptedSnapshotStore struct {
	*MemoryStore
}

func (store *interruptedSnapshotStore) SaveSnapshot(context.Context, model.Snapshot) error {
	return errSnapshotInterrupted
}

func assertRecoveredSnapshot(t *testing.T, owner *DocumentOwner, expected model.Snapshot) {
	t.Helper()
	actual, err := owner.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	actualHash, err := actual.Hash()
	if err != nil {
		t.Fatal(err)
	}
	expectedHash, err := expected.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if actual.Revision != expected.Revision || actualHash != expectedHash {
		t.Fatalf("recovered revision/hash = %d/%s, want %d/%s", actual.Revision, actualHash, expected.Revision, expectedHash)
	}
}
