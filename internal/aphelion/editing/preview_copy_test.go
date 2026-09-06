package editing

import (
	"fmt"
	"reflect"
	"testing"

	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func TestPreviewAllocationIndependentOfReplacedBackground(t *testing.T) {
	var baseline float64
	for _, replaced := range []int{0, 256} {
		m := rotationMap()
		target := m.GetTile(util.Point{X: 2, Y: 1, Z: 1})
		prefab := target.Instances()[0].Prefab()
		for i := 0; i < replaced; i++ {
			target.InstancesAdd(prefab)
			target.Instances()[len(target.Instances())-1].SetStableID(fmt.Sprintf("replaced-%d", i))
		}
		hidden := dmmprefab.New(0, "/obj/hidden", dmvars.FromParent(nil))
		target.InstancesAdd(hidden)
		target.Instances()[len(target.Instances())-1].SetStableID("hidden-target")
		before := m.Copy()
		move, err := NewMove(m, util.Bounds{X1: 1, Y1: 1, X2: 1, Y2: 1}, 1,
			func(path string) bool { return path != "/obj/hidden" }, func(util.Point) error { return nil }, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := move.Preview(util.Point{X: 1}); err != nil {
			t.Fatal(err)
		}
		allocs := testing.AllocsPerRun(20, func() {
			if _, err := move.Preview(util.Point{X: 1}); err != nil {
				t.Fatal(err)
			}
		})
		t.Logf("replaced visible background=%d allocations=%v", replaced, allocs)
		if replaced == 0 {
			baseline = allocs
		} else if allocs > baseline+8 {
			t.Errorf("preview allocated for replaced background: baseline=%v large=%v", baseline, allocs)
		}
		if len(target.Instances()) != 2 || target.Instances()[0].StableID() != "hidden-target" || target.Instances()[1].StableID() != "object-1-1" {
			t.Fatal("preview did not preserve hidden and selected identities/order")
		}
		// A displayed instance must never alias the immutable source/background.
		target.Instances()[0].SetStableID("mutated-display-hidden")
		target.Instances()[1].SetStableID("mutated-display-visible")
		move.Finish(true)
		if !reflect.DeepEqual(m.Copy(), before) {
			t.Fatal("display mutation changed the cancellation snapshot")
		}
	}
}
