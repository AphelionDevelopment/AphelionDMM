package search

import (
	"math/rand"
	"reflect"
	"testing"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func TestGroupedQueryMatchesIndependentScans(t *testing.T) {
	rng := rand.New(rand.NewSource(20260906))
	for trial := 0; trial < 200; trial++ {
		m := &dmmap.Dmm{MaxX: 20, MaxY: 1, MaxZ: 1}
		for x := 1; x <= 20; x++ {
			tile := &dmmap.Tile{Coord: util.Point{X: x, Y: 1, Z: 1}}
			for range rng.Intn(6) {
				tile.InstancesAdd(dmmprefab.New(uint64(rng.Intn(10)), "/obj/test", dmvars.FromParent(nil)))
			}
			m.Tiles = append(m.Tiles, tile)
		}
		ids := make([]uint64, rng.Intn(15))
		for i := range ids {
			ids[i] = uint64(rng.Intn(12))
		}
		var expected []*dmminstance.Instance
		for _, id := range ids {
			for _, tile := range m.Tiles {
				for _, instance := range tile.Instances() {
					if instance.Prefab().Id() == id {
						expected = append(expected, instance)
					}
				}
			}
		}
		before := m.Copy()
		if got := ByPrefabIDs(m, ids); !reflect.DeepEqual(got, expected) {
			t.Fatalf("trial %d grouped results differ", trial)
		}
		if !reflect.DeepEqual(m.Copy(), before) {
			t.Fatal("query mutated map")
		}
	}
	if got := ByPrefabIDs(nil, []uint64{1}); got != nil {
		t.Fatal("nil map has results")
	}
}
