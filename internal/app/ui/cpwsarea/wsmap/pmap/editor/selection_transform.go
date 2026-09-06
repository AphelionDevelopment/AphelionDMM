// APHELION EDIT ADDITION START - SELECTION TRANSFORMS
package editor

import (
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/editing"
	"sdmm/internal/util"
)

// commitSelectionTransform captures every before-state before changing display
// tiles. Submission and history use the same authority as other map edits.
func (e *Editor) commitSelectionTransform(plan editing.Transform, before util.Bounds, label string) (util.Bounds, error) {
	acquired := make([]model.Coord, 0, len(plan.Tiles))
	for _, tile := range plan.Tiles {
		coord := model.Coord{X: tile.Coord.X, Y: tile.Coord.Y, Z: tile.Coord.Z}
		_, alreadyCaptured := e.pendingChanges[coord]
		e.BeginTileChange(tile.Coord)
		if e.collaborationErr != nil {
			// Preflight has not changed display tiles. Release only this call's
			// captures; the invalid-content fault and unrelated edits stay guarded.
			for _, captured := range acquired {
				delete(e.pendingChanges, captured)
			}
			return before, e.collaborationErr
		}
		if !alreadyCaptured {
			acquired = append(acquired, coord)
		}
	}
	for _, tile := range plan.Tiles {
		target := e.dmm.GetTile(tile.Coord)
		target.Set(tile.Instances())
		target.InstancesRegenerate()
	}
	e.CommitOperation(label)
	return plan.Bounds, nil
}

// APHELION EDIT ADDITION END
