package editing

import (
	"fmt"
	"reflect"
	"testing"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func rotationMap() *dmmap.Dmm {
	m := &dmmap.Dmm{MaxX: 4, MaxY: 4, MaxZ: 1}
	for y := 1; y <= 4; y++ {
		for x := 1; x <= 4; x++ {
			tile := &dmmap.Tile{Coord: util.Point{X: x, Y: y, Z: 1}}
			vars := &dmvars.MutableVariables{}
			vars.Put("dir", "1")
			vars.Put("pixel_x", "3")
			vars.Put("pixel_y", "-7")
			vars.Put("unknown", `list("opaque", /missing/type)`)
			tile.InstancesAdd(dmmprefab.New(0, "/obj/missing", vars.ToImmutable()))
			tile.Instances()[0].SetStableID(fmt.Sprintf("object-%d-%d", x, y))
			m.Tiles = append(m.Tiles, tile)
		}
	}
	return m
}

func TestRotationRectangularCoordinatesAndIdentity(t *testing.T) {
	m := rotationMap()
	before := m.Copy()
	plan, err := Rotate(m, util.Bounds{X1: 1, Y1: 1, X2: 3, Y2: 2}, 1, true, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, &before) {
		t.Fatal("planning mutated the source map")
	}
	if plan.Bounds != (util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 3}) {
		t.Fatalf("bounds: %+v", plan.Bounds)
	}
	if len(plan.Tiles) != 8 {
		t.Fatalf("source/destination union contains %d tiles, want 8", len(plan.Tiles))
	}
	applyRotation(m, plan)
	// BYOND coordinates grow northward: the bottom-left cell moves to top-left.
	i := m.GetTile(util.Point{X: 1, Y: 3, Z: 1}).Instances()[0]
	if i.StableID() != "object-1-1" || i.Coord() != (util.Point{X: 1, Y: 3, Z: 1}) {
		t.Fatalf("wrong identity/position: %+v", i)
	}
	v := i.Prefab().Vars()
	if v.ValueV("dir", "") != "4" || v.ValueV("pixel_x", "") != "-7" || v.ValueV("pixel_y", "") != "-3" {
		t.Fatal("orientation was not rotated clockwise")
	}
	if v.ValueV("unknown", "") != `list("opaque", /missing/type)` {
		t.Fatal("opaque variable changed")
	}
	if m.GetTile(util.Point{X: 4, Y: 4, Z: 1}).Instances()[0].StableID() != "object-4-4" {
		t.Fatal("unselected tile changed")
	}
}

func TestRotationFourTurnsAndInverse(t *testing.T) {
	for _, turns := range [][]bool{{true, true, true, true}, {false, false, false, false}, {true, false}} {
		m := rotationMap()
		before := m.Copy()
		for _, clockwise := range turns {
			plan, err := Rotate(m, util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 2}, 1, clockwise, func(string) bool { return true })
			if err != nil {
				t.Fatal(err)
			}
			applyRotation(m, plan)
		}
		for j, tile := range m.Tiles {
			i, original := tile.Instances()[0], before.Tiles[j].Instances()[0]
			if i.StableID() != original.StableID() || i.Coord() != original.Coord() || !reflect.DeepEqual(i.Prefab().Vars(), original.Prefab().Vars()) {
				t.Fatalf("rotation drift at %v", tile.Coord)
			}
		}
	}
}

func TestRotationRejectsBoundsAndOpaqueOrientationWithoutMutation(t *testing.T) {
	for _, badDir := range []string{"", "CUSTOM_DIRECTION"} {
		m := rotationMap()
		area := util.Bounds{X1: 3, Y1: 1, X2: 4, Y2: 3}
		if badDir != "" {
			area = util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 2}
			i := m.Tiles[0].Instances()[0]
			i.SetPrefab(dmmprefab.New(0, i.Prefab().Path(), dmvars.Set(i.Prefab().Vars(), "dir", badDir)))
		}
		before := m.Copy()
		if _, err := Rotate(m, area, 1, true, func(string) bool { return true }); err == nil {
			t.Fatal("unsafe rotation accepted")
		}
		if !reflect.DeepEqual(m, &before) {
			t.Fatal("failed rotation changed source")
		}
	}
}

func TestRotationKeepsHiddenInstancesAndInheritedDefaults(t *testing.T) {
	m := rotationMap()
	parent := &dmvars.MutableVariables{}
	parent.Put("dir", "2")
	for _, tile := range m.Tiles {
		tile.InstancesAdd(dmmprefab.New(0, "/obj/hidden", dmvars.FromParent(nil)))
		tile.Instances()[1].SetStableID("hidden-" + tile.Instances()[0].StableID())
		tile.Instances()[0].SetPrefab(dmmprefab.New(0, "/obj/missing", dmvars.FromParent(parent.ToImmutable())))
	}
	for range 4 {
		plan, err := Rotate(m, util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 2}, 1, true, func(path string) bool { return path != "/obj/hidden" })
		if err != nil {
			t.Fatal(err)
		}
		applyRotation(m, plan)
		for _, tile := range m.Tiles {
			for _, i := range tile.Instances() {
				if i.Prefab().Path() == "/obj/hidden" && i.StableID() != fmt.Sprintf("hidden-object-%d-%d", tile.Coord.X, tile.Coord.Y) {
					t.Fatal("hidden instance moved")
				}
			}
		}
	}
	for _, i := range m.Tiles[0].Instances() {
		if i.Prefab().Path() == "/obj/missing" && i.Prefab().Vars().Len() != 0 {
			t.Fatal("full turn did not restore inherited direction")
		}
	}
}

func applyRotation(m *dmmap.Dmm, plan Rotation) {
	for _, tile := range plan.Tiles {
		m.GetTile(tile.Coord).Set(tile.Instances())
	}
}
