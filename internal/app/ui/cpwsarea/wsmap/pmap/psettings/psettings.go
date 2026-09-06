package psettings

import (
	"sdmm/internal/app/config"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmmap"

	"github.com/SpaiR/imgui-go"
)

type App interface {
	PathsFilter() *dm.PathsFilter

	ConfigRegister(config.Config)
}

type editor interface {
	ActiveLevel() int
	// APHELION EDIT ADDITION START - COLLABORATION
	CanChangeMapSize() bool
	// APHELION EDIT ADDITION END

	Dmm() *dmmap.Dmm
	// APHELION EDIT CHANGE - LOCAL RESIZE - ORIGINAL: CommitMapSizeChange(oldMaxX, oldMaxY, oldMaxZ int)
	ResizeMap(maxX, maxY, maxZ int) error
}

type Panel struct {
	app App

	editor editor

	sessionMapSize *sessionMapSize
	// APHELION EDIT ADDITION START - LOCAL RESIZE
	mapSizeError string
	// APHELION EDIT ADDITION END
	sessionScreenshot *sessionScreenshot
}

var cfg *psettingsConfig

func New(app App, editor editor) *Panel {
	if cfg == nil {
		cfg = loadConfig(app)
	}
	return &Panel{app: app, editor: editor, sessionScreenshot: &sessionScreenshot{}}
}

func (p *Panel) Process() {
	imgui.Dummy(imgui.Vec2{X: p.headerSize()})
	p.showMapSize()
	p.showScreenshot()
}

func (p *Panel) headerSize() float32 {
	return window.PointSize() * 150
}
