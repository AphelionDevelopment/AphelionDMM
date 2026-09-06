package editing

import (
	"fmt"
	"reflect"
	"testing"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/util"
)

func TestPlacementSparseAnchorIdentityAndCancel(t *testing.T) {
	m := rotationMap()
	before := m.Copy()
	// Independent minima are (1,1), although that tile is a hole.
	source := []dmmap.Tile{m.GetTile(util.Point{X: 1, Y: 2, Z: 1}).Copy(), m.GetTile(util.Point{X: 2, Y: 1, Z: 1}).Copy()}
	original := []dmmap.Tile{source[0].Copy(), source[1].Copy()}
	p, err := NewPlacement(m, source, 1, func(string) bool { return true }, func(util.Point) error { return nil }, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Copy(), before) {
		t.Fatal("constructor changed map")
	}
	if _, err := p.Preview(util.Point{}); err != nil {
		t.Fatal(err)
	}
	id := m.GetTile(util.Point{X: 1, Y: 2, Z: 1}).Instances()[0].StableID()
	if id == source[0].Instances()[0].StableID() || id == "" {
		t.Fatal("paste reused source identity")
	}
	for _, shift := range []util.Point{{X: 1}, {X: 2}, {X: 1}, {}} {
		if _, err := p.Preview(shift); err != nil {
			t.Fatal(err)
		}
		if got := m.GetTile(util.Point{X: 1 + shift.X, Y: 2, Z: 1}).Instances()[0]; got.StableID() != id || !reflect.DeepEqual(got.Prefab(), source[0].Instances()[0].Prefab()) {
			t.Fatal("preview changed template identity or variables")
		}
		if len(p.background) != 2 {
			t.Fatalf("retained %d backgrounds for two sparse tiles", len(p.background))
		}
		for _, tile := range before.Tiles {
			if tile.Coord == (util.Point{X: 1 + shift.X, Y: 2, Z: 1}) || tile.Coord == (util.Point{X: 2 + shift.X, Y: 1, Z: 1}) {
				continue
			}
			if !reflect.DeepEqual(m.GetTile(tile.Coord).Copy(), tile.Copy()) {
				t.Fatalf("changed hole or passed-over tile %v", tile.Coord)
			}
		}
	}
	if !reflect.DeepEqual(source, original) {
		t.Fatal("paste mutated clipboard source")
	}
	p.Finish(true)
	if !reflect.DeepEqual(m.Copy(), before) {
		t.Fatal("cancel failed to restore exact map")
	}
}

func TestPlacementFailedCaptureAndBoundsLeavePreviewIntact(t *testing.T) {
	m := rotationMap()
	source := []dmmap.Tile{m.Tiles[0].Copy(), m.Tiles[1].Copy()}
	owned := map[util.Point]bool{}
	fail := util.Point{X: 4, Y: 1, Z: 1}
	p, err := NewPlacement(m, source, 1, func(string) bool { return true }, func(c util.Point) error {
		if c == fail {
			return fmt.Errorf("capture failed")
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
	for _, shift := range []util.Point{{X: 2}, {X: 3}, {Z: 1}, {X: -1}} {
		if _, err := p.Preview(shift); err == nil {
			t.Fatalf("invalid placement accepted: %v", shift)
		}
		if !reflect.DeepEqual(m.Copy(), before) || len(owned) != 2 {
			t.Fatal("failed placement changed display or capture ownership")
		}
	}
	p.Finish(true)
	if len(owned) != 0 {
		t.Fatal("cancel retained captures")
	}
}

func TestPlacementRejectsMalformedTemplatesBeforeCapture(t *testing.T) {
	m := rotationMap()
	good := m.Tiles[0].Copy()
	otherZ := good.Copy()
	otherZ.Coord.Z = 2
	tooWide := good.Copy()
	tooWide.Coord.X = 6
	for _, source := range [][]dmmap.Tile{nil, {good, good}, {good, otherZ}, {good, tooWide}, make([]dmmap.Tile, engine.MaxTileChanges+1)} {
		before := m.Copy()
		_, err := NewPlacement(m, source, 1, func(string) bool { return true }, func(util.Point) error { t.Fatal("captured malformed template"); return nil }, nil, nil)
		if err == nil || !reflect.DeepEqual(m.Copy(), before) {
			t.Fatal("malformed template accepted or changed map")
		}
	}
}

func TestPlacementUnchangedPositionDoesNoWork(t *testing.T) {
	m := rotationMap()
	captures, regenerations := 0, 0
	p, err := NewPlacement(m, []dmmap.Tile{m.Tiles[0].Copy()}, 1, func(string) bool { return true }, func(util.Point) error { captures++; return nil }, func(*dmmap.Tile) { regenerations++ }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Preview(util.Point{X: 1}); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(100, func() {
		coords, err := p.Preview(util.Point{X: 1})
		if err != nil || len(coords) != 0 {
			t.Fatal("unchanged preview did work")
		}
	})
	if allocs != 0 || captures != 1 || regenerations != 1 {
		t.Fatalf("unchanged work: allocs=%v capture=%d regenerate=%d", allocs, captures, regenerations)
	}
}
