// APHELION EDIT ADDITION START - PASTE TRANSFORMS
package tools

import (
	"fmt"
	"sdmm/internal/aphelion/editing"
	"sdmm/internal/util"
)

type pasteTransformer interface {
	TransformPastePlacement(*editing.Move, editing.PlacementTransform, util.Point) (util.Bounds, error)
}

func (t *ToolGrab) CanTransformPlacement(owner editor, z int) bool {
	p := t.placement
	return p != nil && p.owner == owner && ed == owner && !p.move.Closed() && p.move.Level() == z
}

func (t *ToolGrab) TransformPlacement(transform editing.PlacementTransform) error {
	p := t.placement
	if p == nil || !t.CanTransformPlacement(p.owner, p.last.Z) {
		return fmt.Errorf("no active paste placement")
	}
	owner, ok := p.owner.(pasteTransformer)
	if !ok {
		return fmt.Errorf("editor does not support paste transforms")
	}
	area, err := owner.TransformPastePlacement(p.move, transform, util.Point{X: p.last.X - 1, Y: p.last.Y - 1})
	p.err, p.valid = err, err == nil
	if err == nil {
		t.fillStart = util.Point{X: int(area.X1), Y: int(area.Y1), Z: p.move.Level()}
		t.fillArea, t.fillAreaInit = area, area
	}
	return err
}

// APHELION EDIT ADDITION END
