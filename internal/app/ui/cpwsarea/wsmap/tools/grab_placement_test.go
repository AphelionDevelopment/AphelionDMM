package tools

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/aphelion/editing"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/util"
)

type placementCanvas struct {
	point             util.Point
	dragging, focused bool
}

func (c *placementCanvas) HoveredTile() util.Point     { return c.point }
func (c *placementCanvas) LastHoveredTile() util.Point { return c.point }
func (c *placementCanvas) HoverOutOfBounds() bool      { return c.point.X < 1 || c.point.X > 4 }
func (c *placementCanvas) Dragging() bool              { return c.dragging }
func (c *placementCanvas) Active() bool                { return c.focused }

func TestPlacementToolFrameClickAndFocus(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx := imgui.CreateContext(nil)
	defer ctx.Destroy()
	io := imgui.CurrentIO()
	io.SetIniFilename("")
	io.SetDisplaySize(imgui.Vec2{X: 640, Y: 480})
	io.Fonts().TextureDataRGBA32()
	g, owner := lifecycleFixture(t)
	oldCC, oldCS, oldGrab, oldName, oldActive, oldStarted := cc, cs, tools[TNGrab], selectedToolName, active, startedTool
	defer func() {
		cc, cs, tools[TNGrab], selectedToolName, active, startedTool = oldCC, oldCS, oldGrab, oldName, oldActive, oldStarted
	}()
	c := &placementCanvas{point: util.Point{X: 2, Y: 1, Z: 1}}
	cc, cs, tools[TNGrab], selectedToolName, active, startedTool = c, c, g, TNGrab, false, nil
	p, err := editing.NewPlacement(owner.m, []dmmap.Tile{owner.m.Tiles[0].Copy()}, 1, func(string) bool { return true }, func(util.Point) error { return nil }, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.StartPlacement(owner, p, c.point)
	frame := func() { imgui.NewFrame(); process(false); imgui.EndFrame() }
	c.dragging = true
	frame()
	if !g.Placing() || owner.commits != 0 {
		t.Fatal("click outside canvas confirmed placement")
	}
	c.dragging = false
	frame()
	c.focused = true
	io.KeyPress(int(glfw.KeyLeftControl))
	c.dragging = true
	frame()
	if !g.Placing() {
		t.Fatal("modified click confirmed placement")
	}
	io.KeyRelease(int(glfw.KeyLeftControl))
	c.dragging = false
	frame()
	c.point.X = 3
	frame()
	if g.Bounds().X1 != 3 {
		t.Fatal("idle mouse movement did not move placement")
	}
	c.dragging = true
	frame()
	c.dragging = false
	frame()
	if g.Placing() || owner.commits != 1 {
		t.Fatal("canvas click did not confirm once")
	}
}

func TestPlacementCancellationUsesOriginatingEditor(t *testing.T) {
	g, owner := lifecycleFixture(t)
	before := owner.m.Copy()
	p, err := editing.NewPlacement(owner.m, []dmmap.Tile{owner.m.Tiles[0].Copy()}, 1, func(string) bool { return true }, func(util.Point) error { return nil }, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.StartPlacement(owner, p, util.Point{X: 2, Y: 1, Z: 1})
	other := &lifecycleEditor{m: &dmmap.Dmm{}}
	ed = other
	g.Reset()
	if !reflect.DeepEqual(owner.m.Copy(), before) || other.commits != 0 || owner.commits != 0 || g.Placing() {
		t.Fatal("cancel targeted the wrong editor or submitted placement")
	}
}

func TestPlacementInvalidTargetCannotConfirm(t *testing.T) {
	g, owner := lifecycleFixture(t)
	p, err := editing.NewPlacement(owner.m, []dmmap.Tile{owner.m.Tiles[0].Copy(), owner.m.Tiles[1].Copy()}, 1, func(string) bool { return true }, func(util.Point) error { return nil }, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.StartPlacement(owner, p, util.Point{X: 4, Y: 1, Z: 1})
	if g.ConfirmPlacement() || !g.Placing() || g.PlacementError() == nil {
		t.Fatal("out-of-bounds target confirmed")
	}
	g.UpdatePlacement(util.Point{X: 3, Y: 1, Z: 1})
	if !g.ConfirmPlacement() || owner.commits != 1 || !g.HasSelectedArea() {
		t.Fatal("valid target did not commit once and select destination")
	}
}
