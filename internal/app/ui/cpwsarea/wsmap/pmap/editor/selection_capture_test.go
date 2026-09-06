package editor

import (
	"context"
	"reflect"
	"testing"

	"sdmm/internal/aphelion/editing"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func captureEditor(t *testing.T) *Editor {
	t.Helper()
	e := selectionEditor(t)
	for y := 1; y <= 2; y++ {
		for x := 1; x <= 4; x++ {
			if x == 1 && y == 1 {
				continue
			}
			tile := &dmmap.Tile{Coord: util.Point{X: x, Y: y, Z: 1}}
			tile.InstancesSet(e.dmm.Tiles[0].Instances().Prefabs())
			e.dmm.Tiles = append(e.dmm.Tiles, tile)
		}
	}
	e.dmm.MaxX, e.dmm.MaxY = 4, 2
	e.initializeCollaboration()
	e.app = &noopReportingApp{editorTestApp: e.app.(*editorTestApp)}
	return e
}

func TestSelectionCaptureFailureAllowsValidatedRecovery(t *testing.T) {
	for _, action := range []string{"rotation", "mirror", "move"} {
		t.Run(action, func(t *testing.T) {
			e := captureEditor(t)
			authority := e.executor
			before, err := authority.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			e.dmm.GetTile(util.Point{X: 2, Y: 2, Z: 1}).Instances()[2].SetStableID("invalid-later-capture")
			display := e.dmm.Copy()
			area := util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 2}
			switch action {
			case "rotation":
				_, err = e.RotateSelection(area, 1, true)
			case "mirror":
				_, err = e.MirrorSelection(area, 1, editing.MirrorHorizontal)
			case "move":
				_, err = e.BeginSelectionMove(area, 1)
			}
			if err == nil {
				t.Fatal("invalid later capture was accepted")
			}
			if !reflect.DeepEqual(e.dmm, &display) {
				t.Fatal("failed preflight changed display contents")
			}
			if _, err := e.SaveSnapshot(context.Background()); err == nil {
				t.Fatal("invalid capture lost its Save guard")
			}
			if err := e.AttachCollaborationExecutor(authority); err != nil {
				t.Fatalf("failed preflight left orphan captures blocking validated recovery: %v", err)
			}
			after, err := e.SaveSnapshot(context.Background())
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatal("recovery did not preserve the authoritative snapshot")
			}
			if e.app.CommandStorage().HasUndoV("test") {
				t.Fatal("failed capture created history")
			}
		})
	}
}

func TestSelectionRefusalKeepsExistingUncommittedEdit(t *testing.T) {
	for _, action := range []string{"rotation", "mirror", "move"} {
		t.Run(action, func(t *testing.T) {
			e := captureEditor(t)
			instance := e.dmm.GetTile(util.Point{X: 4, Y: 2, Z: 1}).Instances()[2]
			prefab := instance.Prefab()
			e.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, prefab.Path(), dmvars.Set(prefab.Vars(), "dir", "4")))
			area := util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 2}
			var err error
			switch action {
			case "rotation":
				_, err = e.RotateSelection(area, 1, true)
			case "mirror":
				_, err = e.MirrorSelection(area, 1, editing.MirrorHorizontal)
			case "move":
				_, err = e.BeginSelectionMove(area, 1)
			}
			if err == nil {
				t.Fatal("selection action consumed an existing uncommitted edit")
			}
			if err := e.AttachCollaborationExecutor(e.executor); err == nil {
				t.Fatal("selection refusal cleared an unrelated edit's attachment guard")
			}
			e.CommitOperation("Keep original edit")
			after, err := e.SaveSnapshot(context.Background())
			if err != nil || after.Revision != 1 {
				t.Fatal("original edit could not commit after selection refusal")
			}
			if got := e.dmm.GetTile(util.Point{X: 4, Y: 2, Z: 1}).Instances()[2].Prefab().Vars().ValueV("dir", ""); got != "4" {
				t.Fatal("selection refusal discarded the original edit")
			}
		})
	}
}
