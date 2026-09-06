package editing

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"sdmm/internal/aphelion/collab/mapadapter"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func previewMapHash(tb testing.TB, m *dmmap.Dmm) string {
	tb.Helper()
	snapshot, err := mapadapter.Import(m, model.DocumentID("00000000-0000-7000-8000-000000000001"), strings.Repeat("a", 64))
	if err != nil {
		tb.Fatal(err)
	}
	hash, err := snapshot.Hash()
	if err != nil {
		tb.Fatal(err)
	}
	return hash
}

// Ordinary Grab drags share the paste preview path. This matrix includes a
// visible and a hidden instance on every tile, overlapping destinations and
// passed-over restoration. Actual display hashes and exact cancellation are
// checked outside timing, with deterministic fixture identities across binaries.
func BenchmarkMovePreview(b *testing.B) {
	for _, side := range []int{1, 10, 64} {
		b.Run(fmt.Sprintf("cells%d", side*side), func(b *testing.B) {
			m := &dmmap.Dmm{MaxX: side + 2, MaxY: side, MaxZ: 1}
			vars := &dmvars.MutableVariables{}
			vars.Put("dir", "1")
			vars.Put("unknown", `list("opaque", /missing/type)`)
			visible := dmmprefab.New(0, "/obj/benchmark", vars.ToImmutable())
			hidden := dmmprefab.New(0, "/obj/hidden", vars.ToImmutable())
			for y := 1; y <= m.MaxY; y++ {
				for x := 1; x <= m.MaxX; x++ {
					tile := &dmmap.Tile{Coord: util.Point{X: x, Y: y, Z: 1}}
					tile.InstancesAdd(visible)
					tile.InstancesAdd(hidden)
					for i, instance := range tile.Instances() {
						instance.SetStableID(fmt.Sprintf("00000000-0000-7000-8000-%012x", ((y-1)*m.MaxX+x)*2+i))
					}
					m.Tiles = append(m.Tiles, tile)
				}
			}
			before := previewMapHash(b, m)
			move, err := NewMove(m, util.Bounds{X1: 1, Y1: 1, X2: float32(side), Y2: float32(side)}, 1,
				func(path string) bool { return path != "/obj/hidden" }, func(util.Point) error { return nil }, nil, nil)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := move.Preview(util.Point{X: 1}); err != nil {
				b.Fatal(err)
			}
			expectedHash := previewMapHash(b, m)
			runtime.GC()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := move.Preview(util.Point{X: 2 - i%2}); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			if len(move.background) > side*(side+2) {
				b.Fatal("background grew beyond source/destination union")
			}
			if _, err := move.Preview(util.Point{X: 1}); err != nil {
				b.Fatal(err)
			}
			if got := previewMapHash(b, m); got != expectedHash {
				b.Fatal("move display hash changed")
			} else {
				b.Logf("display_sha256=%s", got)
			}
			move.Finish(true)
			if got := previewMapHash(b, m); got != before {
				b.Fatal("cancel did not restore original map hash")
			}
		})
	}
}
