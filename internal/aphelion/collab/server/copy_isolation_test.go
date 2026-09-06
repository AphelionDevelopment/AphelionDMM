package server

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"sdmm/internal/aphelion/collab/model"
)

func TestFailedAppendPreservesExistingTileState(t *testing.T) {
	snapshot := testSnapshot(t, 2)
	seed := testOperation(t, snapshot, 1)
	snapshot.Tiles = []model.Tile{{Coord: seed.Changes[0].Coord, State: model.CloneTileState(seed.Changes[0].After)}}
	snapshot.Tiles[0].State.Prefabs[0].Vars["dir"] = "2"
	store := &failingAppendStore{MemoryStore: NewMemoryStore()}
	owner, err := StartDocument(context.Background(), snapshot, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	operation := testOperation(t, snapshot, 1)
	operation.Changes[0].Before = model.CloneTileState(snapshot.Tiles[0].State)
	operation.Changes[0].After = model.CloneTileState(snapshot.Tiles[0].State)
	operation.Changes[0].After.Prefabs[0].Vars["dir"] = "4"
	if _, err := owner.Submit(context.Background(), operation); !errors.Is(err, errAppendFailed) {
		t.Fatalf("failed append returned %v", err)
	}
	operation.Changes[0].After.Prefabs[0].Vars["dir"] = "caller mutation"
	current, err := owner.Snapshot(context.Background())
	if err != nil || !reflect.DeepEqual(current, snapshot) {
		t.Fatal("failed append or caller mutation changed authoritative contents")
	}
}
