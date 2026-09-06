// APHELION EDIT ADDITION START - PASTE PLACEMENT
package tools

import (
	"fmt"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/aphelion/editing"
	"sdmm/internal/util"
)

type grabPlacement struct {
	owner editor
	move  *editing.Move
	last  util.Point
	err   error
	valid bool
}

func (t *ToolGrab) Placing() bool { return t.placement != nil }
func (t *ToolGrab) PlacementError() error {
	if t.placement == nil {
		return nil
	}
	return t.placement.err
}

func (t *ToolGrab) StartPlacement(owner editor, move *editing.Move, coord util.Point) {
	t.Reset()
	t.placement = &grabPlacement{owner: owner, move: move}
	t.UpdatePlacement(coord)
}

func (t *ToolGrab) UpdatePlacement(coord util.Point) {
	p := t.placement
	if p == nil {
		return
	}
	if p.move.Closed() {
		t.Reset()
		return
	}
	if p.last == coord {
		return
	}
	p.last, p.valid = coord, false
	if coord.Z != p.move.Level() {
		p.err = fmt.Errorf("paste target must be on the selected level")
		return
	}
	area, err := p.owner.PreviewSelectionMove(p.move, util.Point{X: coord.X - 1, Y: coord.Y - 1})
	p.err = err
	if err != nil {
		return
	}
	p.valid = true
	t.fillStart = util.Point{X: int(area.X1), Y: int(area.Y1), Z: p.move.Level()}
	t.fillArea, t.fillAreaInit = area, area
}

func (t *ToolGrab) ConfirmPlacement() bool {
	p := t.placement
	if p == nil || !p.valid || p.move.Closed() || ed != p.owner {
		return false
	}
	// The observer is bound to the originating editor. A later selection has a
	// different history token and cannot be cleared by this operation's outcome.
	t.placement = nil
	t.mode = tSelectModeMoveArea
	t.stopMoveArea()
	history := editing.NewSelectionHistory(t.fillArea)
	t.selectionHistory = history
	changed := func(applied bool) {
		if !applied && ed == p.owner && t.selectionHistory == history && !t.Placing() && !t.dragging {
			t.Reset()
		}
	}
	action := func() error { p.owner.FinishSelectionMove(p.move, false); return nil }
	if observer, ok := p.owner.(selectionTransformObserver); ok {
		_ = observer.TrackSelectionTransform(changed, action)
	} else {
		_ = action()
	}
	return true
}

func (t *ToolGrab) CancelPlacement() {
	if t.Placing() {
		t.Reset()
	}
}

func (t *ToolGrab) processPlacement() {
	if !t.Placing() {
		return
	}
	if t.placement.move.Closed() || ed != t.placement.owner {
		t.Reset()
		return
	}
	if cs == nil || imgui.CurrentIO().WantTextInput() {
		return
	}
	if canvas, ok := cc.(interface{ Active() bool }); ok && !canvas.Active() {
		return
	}
	coord := cs.HoveredTile()
	// Leaving the canvas keeps the last target available to toolbar controls.
	if coord.Z > 0 {
		t.UpdatePlacement(coord)
	}
}

func (t *ToolGrab) clickPlacement(coord util.Point) {
	if canvas, ok := cc.(interface{ Active() bool }); ok && !canvas.Active() {
		return
	}
	if imgui.CurrentIO().WantTextInput() {
		return
	}
	for _, key := range []glfw.Key{glfw.KeyLeftControl, glfw.KeyRightControl, glfw.KeyLeftSuper, glfw.KeyRightSuper, glfw.KeyLeftAlt, glfw.KeyRightAlt, glfw.KeyLeftShift, glfw.KeyRightShift} {
		if imgui.IsKeyDown(int(key)) {
			return
		}
	}
	t.UpdatePlacement(coord)
	t.ConfirmPlacement()
}

// APHELION EDIT ADDITION END
