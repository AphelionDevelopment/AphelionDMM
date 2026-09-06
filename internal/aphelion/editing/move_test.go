package editing

import (
	"reflect"
	"sdmm/internal/util"
	"testing"
)

func TestMoveRestoresRoundTripAndBoundsBackground(t *testing.T) {
	m := rotationMap()
	before := m.Copy()
	move, err := NewMove(m, util.Bounds{X1: 1, Y1: 1, X2: 1, Y2: 1}, 1, func(string) bool { return true }, func(util.Point) error { return nil }, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []int{1, 2, 3, 2, 1, 0} {
		if _, err := move.Preview(util.Point{X: x}); err != nil {
			t.Fatal(err)
		}
		if len(move.background) > 2 {
			t.Fatalf("retained %d background tiles for a one-cell drag", len(move.background))
		}
	}
	if !reflect.DeepEqual(m, &before) {
		t.Fatal("moving back to the start changed contents or instance order/IDs")
	}
	if _, err := move.Preview(util.Point{X: 9}); err == nil {
		t.Fatal("out-of-bounds move succeeded")
	}
	if !reflect.DeepEqual(m, &before) {
		t.Fatal("failed preview changed the map")
	}
	move.Finish(false)
	if _, err := move.Preview(util.Point{X: 1}); err == nil {
		t.Fatal("ended gesture can still mutate map")
	}
}
