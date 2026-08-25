package executor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
)

const (
	testDocumentID      = model.DocumentID("01890f3e-7b5c-7abc-8def-0123456789ab")
	testActorID         = model.ActorID("01890f3e-7b5c-7abc-8def-0123456789ba")
	testEnvironmentHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestLocalExecutorConformance(t *testing.T) {
	t.Parallel()

	local := newTestLocal(t)
	base, err := local.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	operation := testOperation(t, base, "01890f3e-7b5c-7abc-8def-0123456789bb", model.Coord{X: 1, Y: 1, Z: 1}, testTile(), model.TileState{})
	accepted, err := local.Execute(context.Background(), operation)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if accepted.Revision != base.Revision+1 {
		t.Fatalf("accepted revision = %d, want %d", accepted.Revision, base.Revision+1)
	}

	inverse, err := local.BuildInverse(context.Background(), accepted.OperationID)
	if err != nil {
		t.Fatalf("BuildInverse() error = %v", err)
	}
	if inverse.InverseOf == nil || *inverse.InverseOf != accepted.OperationID {
		t.Fatalf("inverse target = %v, want %q", inverse.InverseOf, accepted.OperationID)
	}
	if _, err := local.Execute(context.Background(), inverse); err != nil {
		t.Fatalf("Execute(inverse) error = %v", err)
	}
	result, err := local.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot(result) error = %v", err)
	}
	if !result.Tiles[0].State.Equal(testTile()) {
		t.Fatalf("inverse result = %#v, want %#v", result.Tiles[0].State, testTile())
	}
}

func TestLocalExecutorHonorsCancellation(t *testing.T) {
	t.Parallel()

	local := newTestLocal(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	base := testSnapshot()
	operation := testOperation(t, base, "01890f3e-7b5c-7abc-8def-0123456789bb", model.Coord{X: 1, Y: 1, Z: 1}, testTile(), model.TileState{})
	if _, err := local.Execute(ctx, operation); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context cancellation", err)
	}
	if _, err := local.BuildInverse(ctx, operation.OperationID); !errors.Is(err, context.Canceled) {
		t.Fatalf("BuildInverse() error = %v, want context cancellation", err)
	}
	if _, err := local.Snapshot(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Snapshot() error = %v, want context cancellation", err)
	}
}

func TestLocalExecutorSerializesConcurrentOperations(t *testing.T) {
	t.Parallel()

	local := newTestLocal(t)
	base := testSnapshot()
	operations := []model.Operation{
		testOperation(t, base, "01890f3e-7b5c-7abc-8def-0123456789bb", model.Coord{X: 1, Y: 1, Z: 1}, testTile(), model.TileState{}),
		testOperation(t, base, "01890f3e-7b5c-7abc-8def-0123456789bc", model.Coord{X: 2, Y: 1, Z: 1}, model.TileState{}, model.TileState{Prefabs: []model.PrefabState{{
			StableID: "01890f3e-7b5c-7abc-8def-0123456789ad",
			Path:     "/obj/foo2",
		}}}),
	}

	var wait sync.WaitGroup
	errorsFound := make(chan error, len(operations))
	for _, operation := range operations {
		operation := operation
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := local.Execute(context.Background(), operation)
			errorsFound <- err
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatalf("concurrent Execute() error = %v", err)
		}
	}
	snapshot, err := local.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.Revision != base.Revision+2 {
		t.Fatalf("revision = %d, want %d", snapshot.Revision, base.Revision+2)
	}
}

func newTestLocal(t *testing.T) *Local {
	t.Helper()
	document, err := engine.NewDocument(testSnapshot())
	if err != nil {
		t.Fatalf("engine.NewDocument() error = %v", err)
	}
	local, err := NewLocal(document, testActorID)
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	local.now = func() time.Time { return time.Unix(1, 0) }
	return local
}

func testSnapshot() model.Snapshot {
	return model.Snapshot{
		ProtocolVersion: model.ProtocolVersion,
		SchemaVersion:   model.SchemaVersion,
		DocumentID:      testDocumentID,
		Revision:        4,
		EnvironmentHash: testEnvironmentHash,
		MaxX:            2,
		MaxY:            1,
		MaxZ:            1,
		Tiles: []model.Tile{
			{Coord: model.Coord{X: 1, Y: 1, Z: 1}, State: testTile()},
			{Coord: model.Coord{X: 2, Y: 1, Z: 1}, State: model.TileState{}},
		},
	}
}

func testTile() model.TileState {
	return model.TileState{Prefabs: []model.PrefabState{{
		StableID: "01890f3e-7b5c-7abc-8def-0123456789ac",
		Path:     "/obj/foo1",
		Vars:     map[string]string{"dir": "2"},
	}}}
}

func testOperation(t *testing.T, snapshot model.Snapshot, id model.OperationID, coord model.Coord, before model.TileState, after model.TileState) model.Operation {
	t.Helper()
	baseHash, err := snapshot.Hash()
	if err != nil {
		t.Fatalf("hash test snapshot: %v", err)
	}
	return model.Operation{
		ProtocolVersion: model.ProtocolVersion,
		DocumentID:      snapshot.DocumentID,
		ActorID:         testActorID,
		OperationID:     id,
		BaseRevision:    snapshot.Revision,
		EnvironmentHash: snapshot.EnvironmentHash,
		BaseMapHash:     baseHash,
		Kind:            model.OperationKindTileChange,
		Changes:         []model.TileChange{{Coord: coord, Before: before, After: after}},
	}
}
