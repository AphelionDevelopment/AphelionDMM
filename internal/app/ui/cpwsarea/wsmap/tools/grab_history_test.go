package tools

import (
	"testing"

	"sdmm/internal/util"
)

type outcomeEditor struct {
	*lifecycleEditor
	outcomes []func(bool)
}

func (e *outcomeEditor) TrackSelectionTransform(changed func(bool), action func() error) error {
	e.outcomes = append(e.outcomes, changed)
	return action()
}

func TestGrabLateOutcomePreservesNewDragUntilRelease(t *testing.T) {
	grab, base := lifecycleFixture(t)
	e := &outcomeEditor{lifecycleEditor: base}
	ed = e
	origin := grab.Bounds()
	if err := grab.Nudge(util.Point{X: 1}); err != nil {
		t.Fatal(err)
	}
	grab.onStart(util.Point{X: 2, Y: 1, Z: 1})
	grab.onMove(util.Point{X: 3, Y: 1, Z: 1})
	preview := grab.Bounds()
	e.outcomes[0](false)
	if grab.Stale() || grab.Bounds() != preview {
		t.Fatal("older outcome replaced the active mouse preview")
	}
	grab.onStop(util.Point{X: 3, Y: 1, Z: 1})
	if len(e.outcomes) != 2 || grab.Bounds() != preview {
		t.Fatal("mouse release did not retain its own tracked geometry")
	}
	e.outcomes[1](false)
	if grab.Bounds() != origin {
		t.Fatal("rejected mouse move did not skip the rejected earlier nudge")
	}
	grab.Reset()
	e.outcomes[0](true)
	e.outcomes[1](true)
	if grab.HasSelectedArea() {
		t.Fatal("late outcomes revived a discarded selection")
	}
}
