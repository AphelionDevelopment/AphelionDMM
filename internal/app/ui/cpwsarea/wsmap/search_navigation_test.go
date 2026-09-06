package wsmap

import (
	"testing"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/app/ui/cpsearch"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/editor"
	"sdmm/internal/dmapi/dmmap/dmminstance"
)

type searchWorkspaceApp struct{ current *editor.Editor }

func (a *searchWorkspaceApp) CurrentEditor() *editor.Editor      { return a.current }
func (*searchWorkspaceApp) DoEditInstance(*dmminstance.Instance) {}
func (*searchWorkspaceApp) ShowLayout(string, bool)              {}

func TestSearchNavigationThroughWorkspaceAndRegistry(t *testing.T) {
	ws, _ := newSelectionWorkspace(t)
	activateSelectionWorkspace(t, ws)
	e := ws.Map().Editor()
	before := resizeHash(t, resizeSnapshot(t, e))
	var search cpsearch.Search
	search.Init(&searchWorkspaceApp{current: e})
	search.SetFocused(true)
	t.Cleanup(func() { search.SetFocused(false); search.Free() })
	search.SearchByPath("/obj/foo")
	frame := func() {
		imgui.NewFrame()
		imgui.SetNextWindowSizeV(imgui.Vec2{X: 500, Y: 400}, imgui.ConditionAlways)
		imgui.Begin("Workspace Search navigation")
		search.Process(0)
		imgui.End()
		imgui.EndFrame()
	}
	for range 3 {
		frame()
	}
	pressSelectionShortcut(glfw.KeyF3)
	if e.ActiveLevel() != 1 || len(e.FlickInstance()) == 0 || e.FlickInstance()[len(e.FlickInstance())-1].Instance != e.Dmm().Tiles[0].Instances()[2] {
		t.Fatal("F3 did not jump to first current-map result")
	}
	pressSelectionShortcut(glfw.KeyRightShift, glfw.KeyF3)
	if e.ActiveLevel() != 2 {
		t.Fatal("Shift+F3 did not wrap to last map level")
	}
	for range 3 {
		frame()
	}
	pressSelectionShortcut(glfw.KeyF3)
	if e.ActiveLevel() != 1 {
		t.Fatal("F3 did not wrap to first result")
	}
	if resizeHash(t, resizeSnapshot(t, e)) != before {
		t.Fatal("search navigation changed map authority")
	}
}
