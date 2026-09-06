// APHELION EDIT ADDITION START - SELECTION HISTORY
package tools

import (
	"sdmm/internal/aphelion/editing"
	"sdmm/internal/util"
)

type selectionTransformObserver interface {
	TrackSelectionTransform(func(bool), func() error) error
}

func (t *ToolGrab) trackSelectionTransform(before util.Bounds, discardUnchanged bool, action func() (util.Bounds, error)) error {
	if t.selectionHistory == nil {
		t.selectionHistory = editing.NewSelectionHistory(before)
	}
	history, owner := t.selectionHistory, ed.Dmm()
	change := history.Add(before)
	changed := func(applied bool) {
		change.SetApplied(applied)
		// A new explicit selection or another map owns its own geometry. A newer
		// open mouse gesture keeps its preview until release/cancellation.
		if t.selectionHistory == history && ed.Dmm() == owner && !t.dragging {
			t.refreshSelectionHistory(false)
		}
	}
	apply := func() error {
		area, err := action()
		if err != nil || (discardUnchanged && area == before) {
			change.SetApplied(false)
		} else {
			change.SetBounds(area)
		}
		return err
	}
	var err error
	if observer, ok := ed.(selectionTransformObserver); ok {
		err = observer.TrackSelectionTransform(changed, apply)
	} else {
		err = apply()
	}
	history.DiscardUnappliedTail(change)
	t.refreshSelectionHistory(true)
	return err
}

func (t *ToolGrab) refreshSelectionHistory(refreshContents bool) {
	if t.selectionHistory == nil || !t.HasSelectedArea() {
		return
	}
	area := t.selectionHistory.Bounds()
	if !refreshContents && area == t.fillArea && area == t.fillAreaInit {
		return
	}
	t.fillArea = area
	t.fillStart.X, t.fillStart.Y = int(t.fillArea.X1), int(t.fillArea.Y1)
	t.mode = tSelectModeMoveArea
	t.stopMoveArea()
}

// APHELION EDIT ADDITION END
