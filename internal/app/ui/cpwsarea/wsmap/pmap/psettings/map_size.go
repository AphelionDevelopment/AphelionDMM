package psettings

import (
	"fmt"
	// APHELION EDIT CHANGE - LOCAL RESIZE - ORIGINAL: "math"
	"sdmm/internal/aphelion/collab/model"

	"sdmm/internal/imguiext"
	"sdmm/internal/imguiext/style"
	w "sdmm/internal/imguiext/widget"

	"github.com/SpaiR/imgui-go"
	"github.com/rs/zerolog/log"
)

const (
	// APHELION EDIT CHANGE - LOCAL RESIZE - ORIGINAL: possibleMaxX = math.MaxInt
	possibleMaxX = model.MaxMapDimension
	// APHELION EDIT CHANGE - LOCAL RESIZE - ORIGINAL: possibleMaxY = math.MaxInt
	possibleMaxY = model.MaxMapDimension
	// APHELION EDIT CHANGE - LOCAL RESIZE - ORIGINAL: possibleMaxZ = math.MaxInt
	possibleMaxZ = model.MaxMapDimension
)

type sessionMapSize struct {
	maxX, maxY, maxZ int32
}

func (s sessionMapSize) String() string {
	return fmt.Sprintf("maxX: %d, maxY: %d, maxZ: %d", s.maxX, s.maxY, s.maxZ)
}

func (p *Panel) DropSessionMapSize() {
	p.sessionMapSize = nil
	// APHELION EDIT ADDITION START - LOCAL RESIZE
	p.mapSizeError = ""
	// APHELION EDIT ADDITION END
}

func (p *Panel) showMapSize() {
	if imgui.CollapsingHeader("Map Size") {
		// APHELION EDIT ADDITION START - COLLABORATION
		canChangeMapSize := p.editor.CanChangeMapSize()
		imgui.BeginDisabledV(!canChangeMapSize)
		// APHELION EDIT ADDITION END
		if p.sessionMapSize == nil {
			p.sessionMapSize = &sessionMapSize{
				maxX: int32(p.editor.Dmm().MaxX),
				maxY: int32(p.editor.Dmm().MaxY),
				maxZ: int32(p.editor.Dmm().MaxZ),
			}
		}

		imgui.AlignTextToFramePadding()
		imgui.Text("X")
		imgui.SameLine()
		imgui.SetNextItemWidth(-1)
		imguiext.InputIntClamp("##max_x", &p.sessionMapSize.maxX, 1, possibleMaxX, 1, 10)

		imgui.AlignTextToFramePadding()
		imgui.Text("Y")
		imgui.SameLine()
		imgui.SetNextItemWidth(-1)
		imguiext.InputIntClamp("##max_y", &p.sessionMapSize.maxY, 1, possibleMaxY, 1, 10)

		imgui.AlignTextToFramePadding()
		imgui.Text("Z")
		imgui.SameLine()
		imgui.SetNextItemWidth(-1)
		imguiext.InputIntClamp("##max_z", &p.sessionMapSize.maxZ, 1, possibleMaxZ, 1, 10)

		imgui.Separator()

		w.Button("Set", p.doSetMapSize).
			Size(imgui.Vec2{X: -1}).
			Style(style.ButtonGreen{}).
			Build()
		// APHELION EDIT ADDITION START - COLLABORATION
		imgui.EndDisabled()
		if !canChangeMapSize {
			imgui.TextDisabled("Resize requires an idle local map")
		}
		if p.mapSizeError != "" {
			imgui.TextWrapped(p.mapSizeError)
		}
		// APHELION EDIT ADDITION END
	} else {
		p.sessionMapSize = nil
	}
}

func (p *Panel) doSetMapSize() {
	// APHELION EDIT ADDITION START - COLLABORATION
	if p.sessionMapSize == nil || !p.editor.CanChangeMapSize() {
		return
	}
	// APHELION EDIT ADDITION END
	log.Printf("do set map size [%s]: %v", p.editor.Dmm().Name, p.sessionMapSize)
	/* APHELION EDIT REMOVAL START - LOCAL RESIZE
	oldMaxX, oldMaxY, oldMaxZ := p.editor.Dmm().MaxX, p.editor.Dmm().MaxY, p.editor.Dmm().MaxZ
	p.editor.Dmm().SetMapSize(int(p.sessionMapSize.maxX), int(p.sessionMapSize.maxY), int(p.sessionMapSize.maxZ))
	p.editor.CommitMapSizeChange(oldMaxX, oldMaxY, oldMaxZ)
	APHELION EDIT REMOVAL END */
	// APHELION EDIT ADDITION START - LOCAL RESIZE
	if err := p.editor.ResizeMap(int(p.sessionMapSize.maxX), int(p.sessionMapSize.maxY), int(p.sessionMapSize.maxZ)); err != nil {
		p.mapSizeError = err.Error()
		return
	}
	p.mapSizeError = ""
	// APHELION EDIT ADDITION END
	p.sessionMapSize = nil
}
