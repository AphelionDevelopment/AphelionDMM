package server

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
)

func TestDocumentOwnerSerializesConcurrentOperations(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 32)
	store := NewMemoryStore()
	owner, err := StartDocument(context.Background(), snapshot, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })

	const operationCount = 24
	accepted := make(chan model.AcceptedOperation, operationCount)
	errs := make(chan error, operationCount)
	var workers sync.WaitGroup
	for index := 0; index < operationCount; index++ {
		index := index
		workers.Add(1)
		go func() {
			defer workers.Done()
			operation := testOperation(t, snapshot, index+1)
			result, submitErr := owner.Submit(context.Background(), operation)
			if submitErr != nil {
				errs <- submitErr
				return
			}
			accepted <- result
		}()
	}
	workers.Wait()
	close(errs)
	for submitErr := range errs {
		t.Errorf("Submit() error = %v", submitErr)
	}
	close(accepted)

	var revisions []int
	for result := range accepted {
		revisions = append(revisions, int(result.Revision))
	}
	sort.Ints(revisions)
	if len(revisions) != operationCount {
		t.Fatalf("accepted %d operations, want %d", len(revisions), operationCount)
	}
	for index, revision := range revisions {
		if revision != index+1 {
			t.Fatalf("revision[%d] = %d, want %d", index, revision, index+1)
		}
	}

	storedSnapshot, operations, err := store.Load(context.Background(), snapshot.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if storedSnapshot.Revision != 0 {
		t.Fatalf("stored base revision = %d, want 0", storedSnapshot.Revision)
	}
	if len(operations) != operationCount {
		t.Fatalf("stored operations = %d, want %d", len(operations), operationCount)
	}
	for index, operation := range operations {
		if operation.Revision != model.Revision(index+1) {
			t.Fatalf("stored operation %d revision = %d", index, operation.Revision)
		}
	}
}

func TestDocumentOwnerDuplicateIsIdempotent(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	store := NewMemoryStore()
	owner, err := StartDocument(context.Background(), snapshot, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })

	operation := testOperation(t, snapshot, 1)
	first, err := owner.Submit(context.Background(), operation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := owner.Submit(context.Background(), operation)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != second.Revision || first.OperationID != second.OperationID {
		t.Fatalf("duplicate result = %#v, want %#v", second, first)
	}
	_, operations, err := store.Load(context.Background(), snapshot.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 {
		t.Fatalf("stored operations = %d, want 1", len(operations))
	}
}

func TestDocumentOwnerDoesNotAdvanceWhenAppendFails(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	store := &failingAppendStore{MemoryStore: NewMemoryStore()}
	owner, err := StartDocument(context.Background(), snapshot, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })

	if _, err := owner.Submit(context.Background(), testOperation(t, snapshot, 1)); !errors.Is(err, errAppendFailed) {
		t.Fatalf("Submit() error = %v, want %v", err, errAppendFailed)
	}
	current, err := owner.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != snapshot.Revision {
		t.Fatalf("revision after failed append = %d, want %d", current.Revision, snapshot.Revision)
	}
}

func TestSubmitReconcilesCommittedAppendError(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 2)
	store := &committedThenFailedStore{MemoryStore: NewMemoryStore(), failNext: true}
	owner, err := StartDocument(context.Background(), snapshot, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })

	first, err := owner.Submit(context.Background(), testOperation(t, snapshot, 1))
	if err != nil {
		t.Fatalf("Submit(first) error = %v, want reconciled success", err)
	}
	if first.Revision != 1 {
		t.Fatalf("Submit(first) revision = %d, want 1", first.Revision)
	}
	current, err := owner.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != first.Revision {
		t.Fatalf("snapshot revision = %d, want reconciled revision %d", current.Revision, first.Revision)
	}
	second, err := owner.Submit(context.Background(), testOperation(t, current, 2))
	if err != nil {
		t.Fatalf("Submit(second) error = %v", err)
	}
	if second.Revision != 2 {
		t.Fatalf("Submit(second) revision = %d, want 2", second.Revision)
	}
}

func TestSubmitReconcilesDuplicateAheadOfMemory(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	store := NewMemoryStore()
	owner, err := StartDocument(context.Background(), snapshot, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })

	operation := testOperation(t, snapshot, 1)
	durableDocument, err := engine.NewDocument(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	durable, err := durableDocument.Apply(operation, time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), durable); err != nil {
		t.Fatal(err)
	}
	accepted, duplicate, err := owner.SubmitWithStatus(context.Background(), operation)
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate || accepted.Revision != durable.Revision {
		t.Fatalf("duplicate result = %#v/%t, want revision %d duplicate", accepted, duplicate, durable.Revision)
	}
	current, err := owner.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != durable.Revision {
		t.Fatalf("snapshot revision = %d, want reconciled revision %d", current.Revision, durable.Revision)
	}
}

func TestSubmitRejectsConflictingOperationIDReuse(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 2)
	store := NewMemoryStore()
	owner, err := StartDocument(context.Background(), snapshot, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })

	operation := testOperation(t, snapshot, 1)
	if _, err := owner.Submit(context.Background(), operation); err != nil {
		t.Fatal(err)
	}
	conflict := model.CloneOperation(operation)
	conflict.Changes[0].Coord.X = 2
	if _, err := owner.Submit(context.Background(), conflict); err == nil {
		t.Fatal("Submit(conflicting operation ID) error = nil")
	}
}

func TestDocumentOwnerCloseIsContextAware(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	owner, err := StartDocument(context.Background(), snapshot, NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Snapshot(context.Background()); !errors.Is(err, ErrDocumentClosed) {
		t.Fatalf("Snapshot() error = %v, want %v", err, ErrDocumentClosed)
	}
}

var errAppendFailed = errors.New("append failed")

var errCommittedThenFailed = errors.New("append outcome is unknown")

type failingAppendStore struct {
	*MemoryStore
}

func (store *failingAppendStore) Append(context.Context, model.AcceptedOperation) error {
	return errAppendFailed
}

type committedThenFailedStore struct {
	*MemoryStore
	failNext bool
}

func (store *committedThenFailedStore) Append(ctx context.Context, accepted model.AcceptedOperation) error {
	if err := store.MemoryStore.Append(ctx, accepted); err != nil {
		return err
	}
	if store.failNext {
		store.failNext = false
		return errCommittedThenFailed
	}
	return nil
}

func testSnapshot(t *testing.T, maxX int) model.Snapshot {
	t.Helper()
	documentID, err := model.NewDocumentID()
	if err != nil {
		t.Fatal(err)
	}
	return model.Snapshot{
		ProtocolVersion: model.ProtocolVersion,
		SchemaVersion:   model.SchemaVersion,
		DocumentID:      documentID,
		EnvironmentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		MaxX:            maxX,
		MaxY:            1,
		MaxZ:            1,
	}
}

func testOperation(t *testing.T, snapshot model.Snapshot, x int) model.Operation {
	t.Helper()
	actorID, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	operationID, err := model.NewOperationID()
	if err != nil {
		t.Fatal(err)
	}
	stableID, err := model.NewStableID()
	if err != nil {
		t.Fatal(err)
	}
	baseHash, err := snapshot.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return model.Operation{
		ProtocolVersion: model.ProtocolVersion,
		DocumentID:      snapshot.DocumentID,
		ActorID:         actorID,
		OperationID:     operationID,
		BaseRevision:    snapshot.Revision,
		EnvironmentHash: snapshot.EnvironmentHash,
		BaseMapHash:     baseHash,
		Kind:            model.OperationKindTileChange,
		Changes: []model.TileChange{{
			Coord:  model.Coord{X: x, Y: 1, Z: 1},
			Before: model.TileState{},
			After: model.TileState{Prefabs: []model.PrefabState{{
				StableID: stableID,
				Path:     "/turf/open/floor",
				Vars:     map[string]string{},
			}}},
		}},
	}
}
