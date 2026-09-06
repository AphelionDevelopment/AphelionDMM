// APHELION EDIT ADDITION START - SELECTION NUDGE
package tools

import (
	"fmt"

	"sdmm/internal/util"
)

// Nudge moves a finished selection one grid tile through the same preview and
// operation path as a mouse drag. Pixel/step offsets and directions are unchanged.
func (t *ToolGrab) Nudge(shift util.Point) error {
	if !t.HasSelectedArea() || !t.Stale() {
		return fmt.Errorf("finish selecting or moving the area before nudging")
	}
	if shift.Z != 0 || (shift != (util.Point{X: -1}) && shift != (util.Point{X: 1}) && shift != (util.Point{Y: -1}) && shift != (util.Point{Y: 1})) {
		return fmt.Errorf("nudge must move one tile along one axis")
	}
	return t.trackSelectionTransform(t.fillArea, true, func() (util.Bounds, error) {
		move, err := ed.BeginSelectionMove(t.fillArea, t.fillStart.Z)
		if err != nil {
			return t.fillArea, err
		}
		if _, err := ed.PreviewSelectionMove(move, shift); err != nil {
			ed.FinishSelectionMove(move, true)
			return move.Bounds(), err
		}
		ed.FinishSelectionMove(move, false)
		return move.Bounds(), nil
	})
}

// APHELION EDIT ADDITION END
