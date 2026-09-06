package editing

import (
	"fmt"
	"reflect"
	"runtime"
	"testing"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

// Measures the model template/preview lifecycle only: no capture conversion,
// bucket rebuild, GL upload or network operation. Initialization and identity
// creation are excluded; each timed call rotates and restores/redraws a preview.
func BenchmarkPlacementTransform(b *testing.B) {
	for _, tc := range []struct {
		name        string
		side, cells int
		sparse      bool
	}{
		{"dense1", 1, 1, false}, {"dense100", 10, 100, false}, {"dense4096", 64, 4096, false},
		{"sparse2_extent16", 16, 2, true}, {"sparse2_extent256", 256, 2, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			m := &dmmap.Dmm{MaxX: tc.side, MaxY: tc.side, MaxZ: 1}
			var source []dmmap.Tile
			vars := &dmvars.MutableVariables{}
			vars.Put("dir", "1")
			vars.Put("pixel_x", "3")
			vars.Put("pixel_y", "-7")
			prefab := dmmprefab.New(0, "/obj/benchmark", vars.ToImmutable())
			for y := 1; y <= tc.side; y++ {
				for x := 1; x <= tc.side; x++ {
					tile := &dmmap.Tile{Coord: util.Point{X: x, Y: y, Z: 1}}
					if !tc.sparse || (x == 1 && y == 1) || (x == tc.side && y == tc.side/2) {
						tile.InstancesAdd(prefab)
						tile.Instances()[0].SetStableID(fmt.Sprintf("00000000-0000-7000-8000-%012x", (y-1)*tc.side+x))
						source = append(source, tile.Copy())
					}
					m.Tiles = append(m.Tiles, tile)
				}
			}
			p, err := NewPlacement(m, source, 1, func(string) bool { return true }, func(util.Point) error { return nil }, nil, nil)
			if err != nil {
				b.Fatal(err)
			}
			defer p.Finish(true)
			// Stable fixture identities allow control/candidate display hashes to
			// match across processes. Production placement still generates fresh IDs.
			for i, tile := range p.source {
				tile.Instances()[0].SetStableID(fmt.Sprintf("00000000-0000-7000-8000-%012x", 100000+i))
			}
			original := append([]dmmap.Tile(nil), p.source...)
			if _, err := p.Preview(util.Point{}); err != nil {
				b.Fatal(err)
			}
			expectedHash := previewMapHash(b, m)
			runtime.GC()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := p.TransformPlacement(PlacementRotateRight, util.Point{}); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			if len(p.background) != tc.cells {
				b.Fatal("background ownership grew beyond selection")
			}
			// Complete the turn outside timing, then check all template IDs, geometry
			// and variables. This catches lost work or cumulative orientation drift.
			for range (4 - b.N%4) % 4 {
				if _, err := p.TransformPlacement(PlacementRotateRight, util.Point{}); err != nil {
					b.Fatal(err)
				}
			}
			for i, tile := range p.source {
				got, want := tile.Instances()[0], original[i].Instances()[0]
				if tile.Coord != original[i].Coord || got.StableID() != want.StableID() || !reflect.DeepEqual(got.Prefab().Vars(), want.Prefab().Vars()) {
					b.Fatal("transform cycle changed content/identities")
				}
			}
			if got := previewMapHash(b, m); got != expectedHash {
				b.Fatal("display differs after full transform cycle")
			} else {
				b.Logf("display_sha256=%s", got)
			}
		})
	}
}
