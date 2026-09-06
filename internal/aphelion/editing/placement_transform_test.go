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

func TestPlacementTransformSparseGeometryAndIdentity(t *testing.T) {
	for _, sequence := range [][]PlacementTransform{
		{PlacementRotateRight, PlacementRotateRight, PlacementRotateRight, PlacementRotateRight},
		{PlacementRotateLeft, PlacementRotateRight}, {PlacementMirrorHorizontal, PlacementMirrorHorizontal},
		{PlacementMirrorVertical, PlacementMirrorVertical},
	} {
		m := rotationMap()
		before := m.Copy()
		source := []dmmap.Tile{m.GetTile(util.Point{X: 1, Y: 2, Z: 1}).Copy(), m.GetTile(util.Point{X: 3, Y: 1, Z: 1}).Copy()}
		clipboard := []dmmap.Tile{source[0].Copy(), source[1].Copy()}
		owned := map[util.Point]bool{}
		p, err := NewPlacement(m, source, 1, func(string) bool { return true }, func(c util.Point) error { owned[c] = true; return nil }, nil, func(c util.Point) { delete(owned, c) })
		if err != nil {
			t.Fatal(err)
		}
		original := append([]dmmap.Tile(nil), p.source...)
		shift := util.Point{X: 1, Y: 1}
		for _, transform := range sequence {
			if _, err := p.TransformPlacement(transform, shift); err != nil {
				t.Fatal(err)
			}
			if len(owned) != 2 || len(p.background) != 2 {
				t.Fatal("transform retained passed-over tiles or filled sparse holes")
			}
			for _, tile := range before.Tiles {
				if !owned[tile.Coord] && !reflect.DeepEqual(m.GetTile(tile.Coord).Copy(), tile.Copy()) {
					t.Fatalf("transform changed hole or abandoned tile %v", tile.Coord)
				}
			}
		}
		if p.Bounds() != (util.Bounds{X1: 2, Y1: 2, X2: 4, Y2: 3}) {
			t.Fatalf("inverse geometry drift: %v", p.Bounds())
		}
		for i, tile := range p.source {
			got, want := tile.Instances()[0], original[i].Instances()[0]
			if tile.Coord != original[i].Coord || got.Coord() != want.Coord() || got.StableID() != want.StableID() || !reflect.DeepEqual(got.Prefab().Vars(), want.Prefab().Vars()) {
				t.Fatal("inverse transform changed identity, coordinate or variables")
			}
		}
		if !reflect.DeepEqual(source, clipboard) {
			t.Fatal("transform mutated clipboard")
		}
		p.Finish(true)
		if len(owned) != 0 || !reflect.DeepEqual(m.Copy(), before) {
			t.Fatal("cancel failed exact restoration")
		}
	}
}

func TestPlacementTransformFailureIsAtomic(t *testing.T) {
	m := rotationMap()
	owned := map[util.Point]bool{}
	fail := false
	p, err := NewPlacement(m, []dmmap.Tile{m.Tiles[0].Copy(), m.Tiles[2].Copy()}, 1, func(string) bool { return true }, func(c util.Point) error {
		if fail && c == (util.Point{X: 2, Y: 3, Z: 1}) {
			return fmt.Errorf("injected capture failure")
		}
		owned[c] = true
		return nil
	}, nil, func(c util.Point) { delete(owned, c) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Preview(util.Point{}); err != nil {
		t.Fatal(err)
	}
	before := m.Copy()
	template, bounds := append([]dmmap.Tile(nil), p.source...), p.Bounds()
	fail = true
	for _, trial := range []struct {
		transform PlacementTransform
		shift     util.Point
	}{
		{PlacementRotateRight, util.Point{X: 1}}, // acquire one new tile, then fail
		{PlacementRotateRight, util.Point{Y: 3}}, {PlacementRotateLeft, util.Point{Z: 1}}, {0, util.Point{}},
	} {
		if _, err := p.TransformPlacement(trial.transform, trial.shift); err == nil {
			t.Fatal("invalid transform accepted")
		}
		if !reflect.DeepEqual(m.Copy(), before) || !reflect.DeepEqual(p.source, template) || p.Bounds() != bounds || len(owned) != 2 || len(p.background) != 2 {
			t.Fatal("failed transform changed template, display or journal ownership")
		}
	}
	fail = false
	if _, err := p.TransformPlacement(PlacementRotateRight, util.Point{X: 1}); err != nil {
		t.Fatal(err)
	}
	// New orientation at the same target must retain the stationary fast path.
	if allocs := testing.AllocsPerRun(100, func() {
		coords, err := p.Preview(util.Point{X: 1})
		if err != nil || len(coords) != 0 {
			t.Fatal("stationary preview changed")
		}
	}); allocs != 0 {
		t.Fatalf("stationary allocations: %v", allocs)
	}
	p.Finish(true)
	if _, err := p.TransformPlacement(PlacementRotateRight, util.Point{}); err == nil {
		t.Fatal("closed placement transformed")
	}
}

func TestPlacementTransformRejectsExpressionsBeforeCapture(t *testing.T) {
	m := rotationMap()
	vars := &dmvars.MutableVariables{}
	vars.Put("dir", "dynamic_direction()")
	m.Tiles[0].Instances()[0].SetPrefab(dmmprefab.New(0, "/obj/missing", vars.ToImmutable()))
	p, err := NewPlacement(m, []dmmap.Tile{m.Tiles[0].Copy()}, 1, func(string) bool { return true }, func(util.Point) error { t.Fatal("captured invalid transform"); return nil }, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := m.Copy()
	for _, transform := range []PlacementTransform{PlacementRotateRight, PlacementRotateLeft, PlacementMirrorHorizontal, PlacementMirrorVertical} {
		if _, err := p.TransformPlacement(transform, util.Point{}); err == nil || !reflect.DeepEqual(m.Copy(), before) {
			t.Fatal("invalid orientation accepted or changed map")
		}
	}
}
