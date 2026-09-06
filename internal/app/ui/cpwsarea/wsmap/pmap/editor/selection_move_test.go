package editor

import (
	"context"
	"reflect"
	"sdmm/internal/aphelion/collab/mapadapter"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/app/command"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmsnap"
	"sdmm/internal/util"
	"testing"
)

func selectionEditor(t testing.TB) *Editor {
	t.Helper()
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)
	environment := editorTestEnvironment()
	m := editorTestMap(environment)
	app := &editorTestApp{commands: command.NewStorage(), environment: environment, paths: dm.NewPathsFilterEmpty()}
	app.commands.SetStack("test")
	return New(app, &editorTestAttachedMap{snapshot: dmmsnap.New(m)}, m)
}

func TestSelectionMoveBlocksMapResize(t *testing.T) {
	e := selectionEditor(t)
	if !e.CanChangeMapSize() {
		t.Fatal("idle local map cannot resize")
	}
	if _, err := e.BeginSelectionMove(util.Bounds{X1: 1, Y1: 1, X2: 1, Y2: 1}, 1); err != nil {
		t.Fatal(err)
	}
	if e.CanChangeMapSize() {
		t.Fatal("map resize can replace the map during a drag")
	}
}

func TestSelectionMoveEndsWithAttachment(t *testing.T) {
	e := selectionEditor(t)
	move, err := e.BeginSelectionMove(util.Bounds{X1: 1, Y1: 1, X2: 1, Y2: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	e.Close()
	if _, err := move.Preview(util.Point{}); err == nil {
		t.Fatal("closing the editor left a mutable drag handle")
	}
}

func TestSelectionMoveReleasesRestoredJournalTiles(t *testing.T) {
	e := selectionEditor(t)
	// Extend the map before starting a new local attachment. Preview is exercised
	// directly here to inspect its capture callbacks without an OpenGL context;
	// the real workspace suite separately exercises the editor render adapter.
	for x := 2; x <= 128; x++ {
		tile := &dmmap.Tile{Coord: util.Point{X: x, Y: 1, Z: 1}}
		tile.InstancesSet(e.dmm.Tiles[0].Instances().Prefabs())
		e.dmm.Tiles = append(e.dmm.Tiles, tile)
	}
	e.dmm.MaxX = 128
	e.initializeCollaboration()
	before := model.CloneSnapshot(e.authoritative)
	area := util.Bounds{X1: 1, Y1: 1, X2: 1, Y2: 1}
	move, err := e.BeginSelectionMove(area, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []int{1, 2, 63, 127, 63, 1, 0} {
		if _, err := move.Preview(util.Point{X: x}); err != nil {
			t.Fatal(err)
		}
		if got := len(e.pendingChanges); got > 2 || got == 0 {
			t.Fatalf("one-cell preview at shift %d retains %d journal tiles; want source/destination only", x, got)
		}
		if e.CanChangeMapSize() {
			t.Fatal("pruning journal released the active gesture guard")
		}
		if _, err := e.CollaborationSnapshot(context.Background()); err == nil {
			t.Fatal("open preview allowed snapshot")
		}
	}
	// Returning through old destinations must recapture their exact current
	// before-states, and returning home must preserve every tile and stable ID.
	after, err := mapadapter.Import(e.dmm, before.DocumentID, before.EnvironmentHash)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.Tiles, before.Tiles) {
		t.Fatal("pruning journal changed restored map")
	}
	move.Finish(false)
	e.selectionMove = nil
	e.CommitOperation("No-op return")
	if len(e.pendingChanges) != 0 || e.app.CommandStorage().HasUndoV("test") {
		t.Fatal("round trip left pending work or history")
	}
}
