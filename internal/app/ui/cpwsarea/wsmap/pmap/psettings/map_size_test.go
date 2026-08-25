package psettings

import (
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
}

func (*mapSizeTestEditor) ActiveLevel() int { return 1 }

func (editor *mapSizeTestEditor) Dmm() *dmmap.Dmm { return editor.mapState }

func (editor *mapSizeTestEditor) CommitMapSizeChange(int, int, int) { editor.commits++ }

func (editor *mapSizeTestEditor) CanChangeMapSize() bool { return editor.canChangeMapSize }
