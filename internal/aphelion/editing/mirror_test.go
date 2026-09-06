package editing

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func TestMirrorKeepsHiddenOrderAndInheritedDefaults(t *testing.T) {
	for _, axis := range []MirrorAxis{MirrorHorizontal, MirrorVertical} {
		m := rotationMap()
		parent := &dmvars.MutableVariables{}
		parent.Put("dir", "5")
		parent.Put("pixel_x", "3")
		parent.Put("pixel_y", "7")
		prefab := dmmprefab.New(0, "/obj/missing", dmvars.FromParent(parent.ToImmutable()))
		for _, tile := range m.Tiles {
			tile.Instances()[0].SetPrefab(prefab)
			// Visible/hidden instances start interleaved. A mirror preserves each
			// group's relative order, retaining hidden instances at the destination.
			for index, path := range []string{"/obj/hidden", "/obj/missing", "/obj/hidden"} {
				vars := prefab.Vars()
				if path == "/obj/hidden" {
					vars = dmvars.Set(vars, "dir", "OPAQUE_HIDDEN_DIRECTION")
				}
				tile.InstancesAdd(dmmprefab.New(0, path, vars))
				tile.Instances()[index+1].SetStableID(fmt.Sprintf("%d-%d-%d", tile.Coord.X, tile.Coord.Y, index+1))
			}
		}
		area := util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 2}
		for turn := 1; turn <= 2; turn++ {
			plan, err := Mirror(m, area, 1, axis, func(path string) bool { return path != "/obj/hidden" })
			if err != nil {
				t.Fatal(err)
			}
			applyRotation(m, plan)
			for _, tile := range plan.Tiles {
				instances := m.GetTile(tile.Coord).Instances()
				if len(instances) != 4 || instances[0].StableID() != fmt.Sprintf("%d-%d-1", tile.Coord.X, tile.Coord.Y) || instances[1].StableID() != fmt.Sprintf("%d-%d-3", tile.Coord.X, tile.Coord.Y) {
					t.Fatal("mirror lost hidden contents, their position or relative order")
				}
				x, y := tile.Coord.X, tile.Coord.Y
				if turn == 1 {
					if axis == MirrorHorizontal {
						x = 3 - x
					} else {
						y = 3 - y
					}
				}
				if instances[2].StableID() != fmt.Sprintf("object-%d-%d", x, y) || instances[3].StableID() != fmt.Sprintf("%d-%d-2", x, y) {
					t.Fatal("mirror lost visible contents or relative order")
				}
				for _, instance := range instances {
					if instance.Coord() != tile.Coord {
						t.Fatal("mirror retained a source coordinate")
					}
				}
				for _, instance := range instances[2:] {
					if turn == 2 && instance.Prefab().Vars().Len() != 0 {
						t.Fatal("second mirror did not restore inherited orientation defaults")
					}
				}
			}
		}
	}
}

func TestMirrorRectangularCoordinatesIdentityAndOrientation(t *testing.T) {
	for _, axis := range []MirrorAxis{MirrorHorizontal, MirrorVertical} {
		t.Run(map[MirrorAxis]string{MirrorHorizontal: "horizontal", MirrorVertical: "vertical"}[axis], func(t *testing.T) {
			m := rotationMap()
			before := m.Copy()
			area := util.Bounds{X1: 2, Y1: 1, X2: 4, Y2: 2}
			plan, err := Mirror(m, area, 1, axis, func(string) bool { return true })
			if err != nil {
				t.Fatal(err)
			}
			if plan.Bounds != area || len(plan.Tiles) != 6 || !reflect.DeepEqual(m, &before) {
				t.Fatal("mirror planning changed source, bounds or footprint")
			}
			applyRotation(m, plan)
			destination := util.Point{X: 4, Y: 1, Z: 1}
			wantDir, wantX, wantY := "1", "-3", "-7"
			if axis == MirrorVertical {
				destination = util.Point{X: 2, Y: 2, Z: 1}
				wantDir, wantX, wantY = "2", "3", "7"
			}
			instance := m.GetTile(destination).Instances()[0]
			vars := instance.Prefab().Vars()
			if instance.StableID() != "object-2-1" || instance.Coord() != destination || vars.ValueV("dir", "") != wantDir || vars.ValueV("pixel_x", "") != wantX || vars.ValueV("pixel_y", "") != wantY {
				t.Fatal("mirror changed the wrong coordinate, identity or orientation")
			}
			if vars.ValueV("unknown", "") != `list("opaque", /missing/type)` {
				t.Fatal("mirror changed an opaque variable")
			}
			plan, err = Mirror(m, area, 1, axis, func(string) bool { return true })
			if err != nil {
				t.Fatal(err)
			}
			applyRotation(m, plan)
			for index, tile := range m.Tiles {
				got, want := tile.Instances()[0], before.Tiles[index].Instances()[0]
				if got.StableID() != want.StableID() || got.Coord() != want.Coord() || !reflect.DeepEqual(got.Prefab().Vars(), want.Prefab().Vars()) {
					t.Fatal("two mirrors changed content, identity or unselected tiles")
				}
			}
		})
	}
}

func TestMirrorDirectionAndOffsetAxes(t *testing.T) {
	for _, test := range []struct {
		axis                                        MirrorAxis
		dir, wantDir, offset, wantOffset, untouched string
	}{
		{MirrorHorizontal, "NORTHEAST", "9", "step_x", "-5", "step_y"},
		{MirrorVertical, "NORTHWEST", "10", "step_y", "-5", "step_x"},
	} {
		m := rotationMap()
		instance := m.Tiles[0].Instances()[0]
		vars := dmvars.Set(instance.Prefab().Vars(), "dir", test.dir)
		vars = dmvars.Set(vars, test.offset, "5")
		vars = dmvars.Set(vars, test.untouched, "OPAQUE_UNCHANGED_EXPRESSION")
		instance.SetPrefab(dmmprefab.New(0, instance.Prefab().Path(), vars))
		plan, err := Mirror(m, util.Bounds{X1: 1, Y1: 1, X2: 1, Y2: 1}, 1, test.axis, func(string) bool { return true })
		if err != nil {
			t.Fatal(err)
		}
		got := plan.Tiles[0].Instances()[0].Prefab().Vars()
		if got.ValueV("dir", "") != test.wantDir || got.ValueV(test.offset, "") != test.wantOffset || got.ValueV(test.untouched, "") != "OPAQUE_UNCHANGED_EXPRESSION" {
			t.Fatal("mirror changed the wrong offset axis or direction bits")
		}
	}
}

func TestMirrorRejectsInvalidInputWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		area            util.Bounds
		axis            MirrorAxis
		variable, value string
	}{
		{util.Bounds{X1: 0, Y1: 1, X2: 2, Y2: 2}, MirrorHorizontal, "", ""},
		{util.Bounds{X1: float32(math.NaN()), Y1: 1, X2: 2, Y2: 2}, MirrorHorizontal, "", ""},
		{util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 2}, MirrorAxis(99), "", ""},
		{util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 2}, MirrorHorizontal, "dir", "GAME_SPECIFIC_DIR"},
		{util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 2}, MirrorVertical, "pixel_y", "NaN"},
		{util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 2}, MirrorHorizontal, "step_x", "CUSTOM_OFFSET"},
	} {
		m := rotationMap()
		if test.variable != "" {
			i := m.GetTile(util.Point{X: 2, Y: 2, Z: 1}).Instances()[0]
			i.SetPrefab(dmmprefab.New(0, i.Prefab().Path(), dmvars.Set(i.Prefab().Vars(), test.variable, test.value)))
		}
		before := m.Copy()
		if _, err := Mirror(m, test.area, 1, test.axis, func(string) bool { return true }); err == nil {
			t.Fatal("invalid mirror was accepted")
		}
		if !reflect.DeepEqual(m, &before) {
			t.Fatal("failed mirror partially mutated source")
		}
	}
}
