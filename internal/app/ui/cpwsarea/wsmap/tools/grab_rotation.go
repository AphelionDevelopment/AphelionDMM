// APHELION EDIT ADDITION START - SELECTION ROTATION
package tools

import (
	"fmt"
	"sdmm/internal/util"
)

func (t *ToolGrab) SelectionLevel() int { return t.fillStart.Z }

// Rotate keeps selection bookkeeping in sync with a committed editor transform.
func (t *ToolGrab) Rotate(clockwise bool, transform func(util.Bounds, int, bool) (util.Bounds, error)) error {
	if !t.HasSelectedArea() || !t.Stale() {
		return fmt.Errorf("finish selecting or moving the area before rotating")
	}
	return t.trackSelectionTransform(t.fillArea, false, func() (util.Bounds, error) {
		return transform(t.fillArea, t.fillStart.Z, clockwise)
	})
}

// APHELION EDIT ADDITION END
