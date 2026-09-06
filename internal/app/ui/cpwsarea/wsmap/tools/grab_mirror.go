// APHELION EDIT ADDITION START - SELECTION MIRROR
package tools

import (
	"fmt"

	"sdmm/internal/aphelion/editing"
	"sdmm/internal/util"
)

func (t *ToolGrab) Mirror(axis editing.MirrorAxis, transform func(util.Bounds, int, editing.MirrorAxis) (util.Bounds, error)) error {
	if !t.HasSelectedArea() || !t.Stale() {
		return fmt.Errorf("finish selecting or moving the area before mirroring")
	}
	return t.trackSelectionTransform(t.fillArea, false, func() (util.Bounds, error) {
		return transform(t.fillArea, t.fillStart.Z, axis)
	})
}

// APHELION EDIT ADDITION END
