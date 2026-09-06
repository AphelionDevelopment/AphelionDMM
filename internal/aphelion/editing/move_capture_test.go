package editing

import (
	"errors"
	"reflect"
	"testing"

	"sdmm/internal/util"
)

func TestMoveFailedDestinationCaptureKeepsPreviousPreview(t *testing.T) {
	m := rotationMap()
	original := m.Copy()
	failure := errors.New("controlled later destination capture")
	failAt := util.Point{X: 4, Y: 3, Z: 1}
	captured := make(map[util.Point]bool)
	move, err := NewMove(m, util.Bounds{X1: 1, Y1: 1, X2: 2, Y2: 1}, 1,
		func(string) bool { return true }, func(coord util.Point) error {
			if coord == failAt {
				return failure
			}
			captured[coord] = true
			return nil
		}, nil, func(coord util.Point) { delete(captured, coord) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := move.Preview(util.Point{Y: 1}); err != nil {
		t.Fatal(err)
	}
	previous := m.Copy()
	if _, err := move.Preview(util.Point{X: 2, Y: 2}); !errors.Is(err, failure) {
		t.Fatal("later destination failure was not returned")
	}
	if !reflect.DeepEqual(m.Copy(), previous) || move.Bounds() != (util.Bounds{X1: 1, Y1: 2, X2: 2, Y2: 2}) {
		t.Fatal("failed capture replaced previous preview or bounds")
	}
	// The first destination in the failed batch belongs to neither preview.
	// It must be recaptured if the caller retries, instead of restoring this
	// stale copy over a later update.
	newDestination := util.Point{X: 3, Y: 3, Z: 1}
	if captured[newDestination] {
		t.Error("failed preview retained a new destination capture")
	}
	m.GetTile(newDestination).Instances()[0].SetStableID("new-destination-state")
	failAt = util.Point{}
	if _, err := move.Preview(util.Point{X: 2, Y: 2}); err != nil {
		t.Fatal(err)
	}
	move.Finish(true)
	original.GetTile(newDestination).Instances()[0].SetStableID("new-destination-state")
	if !reflect.DeepEqual(m.Copy(), original) {
		t.Fatal("retry/cancel restored a stale failed-destination background")
	}
	if len(captured) != 0 {
		t.Fatal("cancelled move retained capture ownership")
	}
}
