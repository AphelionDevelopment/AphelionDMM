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

func TestEditorMirrorRejectsWholeTransformBeforeChangingDisplay(t *testing.T) {
	for _, fault := range []string{"orientation", "identity"} {
		t.Run(fault, func(t *testing.T) {
			e := selectionEditor(t)
			tile := &dmmap.Tile{Coord: util.Point{X: 2, Y: 1, Z: 1}}
			tile.InstancesSet(e.dmm.Tiles[0].Instances().Prefabs())
			e.dmm.Tiles = append(e.dmm.Tiles, tile)
			e.dmm.MaxX = 2
			e.initializeCollaboration()
			if e.collaborationErr != nil {
				t.Fatal(e.collaborationErr)
			}
			instance := e.dmm.GetTile(util.Point{X: 2, Y: 1, Z: 1}).Instances()[2]
			if fault == "orientation" {
				prefab := instance.Prefab()
				instance.SetPrefab(dmmprefab.New(0, prefab.Path(), dmvars.Set(prefab.Vars(), "dir", "UNSUPPORTED_DIRECTION")))
			} else {
				instance.SetStableID("invalid-identity")
			}
			before := e.dmm.Copy()
			area := util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 1}
			if _, err := e.MirrorSelection(area, 1, editing.MirrorHorizontal); err == nil {
				t.Fatal("mirror accepted an invalid tile")
			}
			if !reflect.DeepEqual(e.dmm, &before) || e.app.CommandStorage().HasUndoV("test") {
				t.Fatal("rejected transform partially changed display or history")
			}
			snapshot, err := e.executor.Snapshot(context.Background())
			if err != nil || snapshot.Revision != 0 {
				t.Fatal("rejected transform changed authority")
			}
		})
	}
}
