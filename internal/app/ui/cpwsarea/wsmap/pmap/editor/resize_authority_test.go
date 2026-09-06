package editor

import (
	"errors"
	"testing"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/util"
)

func TestMapResizeRequiresHealthyAuthority(t *testing.T) {
	for _, fault := range []string{"closed", "capture error"} {
		t.Run(fault, func(t *testing.T) {
			e := selectionEditor(t)
			if fault == "closed" {
				e.Close()
			} else {
				e.collaborationErr = errors.New("controlled capture failure")
			}
			if e.CanChangeMapSize() {
				t.Fatal("resize is enabled without healthy authority")
			}
		})
	}
}

func TestUnchangedResizeHasNoAllocationOrHistory(t *testing.T) {
	for _, cells := range []int{100, 1000, 10000} {
		e := selectionEditor(t)
		prefabs := e.dmm.Tiles[0].Instances().Prefabs()
		e.dmm.MaxX, e.dmm.MaxY = 100, cells/100
		e.dmm.Tiles = make([]*dmmap.Tile, cells)
		for index := range e.dmm.Tiles {
			tile := &dmmap.Tile{Coord: util.Point{X: index%100 + 1, Y: index/100 + 1, Z: 1}}
			tile.InstancesSet(prefabs)
			e.dmm.Tiles[index] = tile
		}
		e.initializeCollaboration()
		if e.collaborationErr != nil {
			t.Fatal(e.collaborationErr)
		}
		generation, revision := e.SaveVersion()
		first := e.dmm.Tiles[0]
		var resizeErr error
		allocations := testing.AllocsPerRun(20, func() { resizeErr = e.ResizeMap(100, cells/100, 1) })
		if resizeErr != nil || allocations != 0 {
			t.Fatalf("%d-cell unchanged resize: allocations=%v error=%v", cells, allocations, resizeErr)
		}
		gotGeneration, gotRevision := e.SaveVersion()
		if first != e.dmm.Tiles[0] || gotGeneration != generation || gotRevision != revision || e.app.CommandStorage().HasUndoV("test") {
			t.Fatal("unchanged resize altered map, authority or history")
		}
	}
}
