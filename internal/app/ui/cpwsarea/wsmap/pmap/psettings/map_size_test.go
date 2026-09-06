package psettings

import (
	"errors"
	"testing"

	"sdmm/internal/dmapi/dmmap"
)

func TestSetMapSizeRefusesUnsupportedCollaborationResize(t *testing.T) {
	mapState := &dmmap.Dmm{MaxX: 5, MaxY: 6, MaxZ: 1}
	editor := &mapSizeTestEditor{mapState: mapState, canChangeMapSize: false}
	panel := &Panel{
		editor:         editor,
		sessionMapSize: &sessionMapSize{maxX: 8, maxY: 9, maxZ: 2},
	}

	panel.doSetMapSize()

	if mapState.MaxX != 5 || mapState.MaxY != 6 || mapState.MaxZ != 1 {
		t.Fatalf("collaboration resize changed map size to %dx%dx%d", mapState.MaxX, mapState.MaxY, mapState.MaxZ)
	}
	if editor.commits != 0 {
		t.Fatalf("collaboration resize created %d legacy commits", editor.commits)
	}
}

type mapSizeTestEditor struct {
	mapState         *dmmap.Dmm
	canChangeMapSize bool
	commits          int
	requested        [3]int
	failure          error
}

func (*mapSizeTestEditor) ActiveLevel() int { return 1 }

func (editor *mapSizeTestEditor) Dmm() *dmmap.Dmm { return editor.mapState }

func (editor *mapSizeTestEditor) ResizeMap(x, y, z int) error {
	editor.commits++
	editor.requested = [3]int{x, y, z}
	return editor.failure
}

func (editor *mapSizeTestEditor) CanChangeMapSize() bool { return editor.canChangeMapSize }

func TestSetMapSizeDelegatesWithoutMutatingDisplay(t *testing.T) {
	for _, fail := range []bool{false, true} {
		editor := &mapSizeTestEditor{mapState: &dmmap.Dmm{MaxX: 1, MaxY: 1, MaxZ: 1}, canChangeMapSize: true}
		if fail {
			editor.failure = errors.New("controlled resize failure")
		}
		request := &sessionMapSize{maxX: 8, maxY: 9, maxZ: 2}
		panel := &Panel{editor: editor, sessionMapSize: request}
		panel.doSetMapSize()
		if editor.commits != 1 || editor.requested != ([3]int{8, 9, 2}) || editor.mapState.MaxX != 1 {
			t.Fatal("settings did not delegate before map mutation")
		}
		if fail && (panel.sessionMapSize != request || panel.mapSizeError != editor.failure.Error()) {
			t.Fatal("failed resize lost its input or error")
		}
		if !fail && (panel.sessionMapSize != nil || panel.mapSizeError != "") {
			t.Fatal("successful resize retained stale request state")
		}
	}
}
