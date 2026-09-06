// APHELION EDIT ADDITION START - PASTE TRANSFORMS
package pmap

import "sdmm/internal/app/ui/cpwsarea/wsmap/tools"

func (p *PaneMap) canTransformSelection() bool {
	if p.canRotateSelection() {
		return true
	}
	g, ok := tools.Selected().(*tools.ToolGrab)
	return (activePane == p || (activePane == nil && lastActivePane == p)) && ok && p.editor.HasPastePlacement() && g.CanTransformPlacement(p.editor, p.activeLevel)
}

// APHELION EDIT ADDITION END
