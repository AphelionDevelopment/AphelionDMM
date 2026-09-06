// APHELION EDIT ADDITION START - PASTE PLACEMENT
package editor

import (
	"fmt"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/editing"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/util"
)

func (e *Editor) HasPastePlacement() bool {
	return e.selectionMove != nil && e.selectionMove.IsPlacement()
}

func (e *Editor) startPastePlacement() {
	if e.HasPastePlacement() {
		return
	}
	data := e.app.Clipboard().Buffer()
	if len(data.Buffer) == 0 {
		return
	}
	if e.executor == nil || e.collaborationErr != nil || len(e.pendingChanges) != 0 || e.selectionMove != nil {
		e.reportCollaborationError("Unable to paste", fmt.Errorf("finish or cancel the current edit before pasting"))
		return
	}
	filter := data.Filter.Copy()
	p, err := editing.NewPlacement(e.dmm, data.Buffer, e.pMap.ActiveLevel(), filter.IsVisiblePath,
		func(c util.Point) error {
			if _, exists := e.pendingChanges[model.Coord{X: c.X, Y: c.Y, Z: c.Z}]; exists {
				return fmt.Errorf("paste destination belongs to another edit")
			}
			e.BeginTileChange(c)
			return e.collaborationErr
		}, func(tile *dmmap.Tile) { tile.InstancesRegenerate() },
		func(c util.Point) { delete(e.pendingChanges, model.Coord{X: c.X, Y: c.Y, Z: c.Z}) })
	if err != nil {
		e.reportCollaborationError("Unable to paste", err)
		return
	}
	g := tools.SetSelected(tools.TNGrab).(*tools.ToolGrab)
	g.Reset()
	e.selectionMove, e.selectionMoveGeneration = p, e.attachmentGeneration
	coord := e.pMap.CanvasState().LastHoveredTile()
	coord.Z = e.pMap.ActiveLevel()
	g.StartPlacement(e, p, coord)
}

// APHELION EDIT ADDITION END
