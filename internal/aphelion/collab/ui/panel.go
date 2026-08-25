package ui

import (
	"fmt"

	"sdmm/internal/app/ui/component"
	w "sdmm/internal/imguiext/widget"

	"github.com/SpaiR/imgui-go"
)

type PanelApp interface {
	CollaborationViewModel() ViewModel
	HasActiveCollaboration() bool
	DoLeaveCollaborationSession()
}

type Panel struct {
	component.Component

	app PanelApp
}

func (panel *Panel) Init(app PanelApp) {
	panel.app = app
}

func (panel *Panel) Process(int32) {
	if !panel.app.HasActiveCollaboration() {
		imgui.TextDisabled("No active collaboration session")
		return
	}
	view := panel.app.CollaborationViewModel()
	sessionLabel := view.SessionLabel
	if sessionLabel == "" {
		sessionLabel = "Pending"
	}
	imgui.Text("Session: " + sessionLabel)
	imgui.Text("Role: " + view.RoleLabel)
	imgui.Text(view.RevisionLabel)
	imgui.Text("Status: " + view.SyncLabel)
	if view.ErrorText != "" {
		imgui.Separator()
		imgui.TextWrapped("Error: " + view.ErrorText)
	}

	imgui.Separator()
	imgui.Text("Participants")
	if len(view.Participants) == 0 {
		imgui.TextDisabled("No participant details available")
	}
	for _, participant := range view.Participants {
		label := participant.Label
		if participant.Status != "" {
			label += " - " + participant.Status
		}
		imgui.BulletText(label)
	}

	if len(view.ConflictSummaries) != 0 || view.HiddenConflictCount != 0 {
		imgui.Separator()
		imgui.Text("Conflicts")
		for _, summary := range view.ConflictSummaries {
			imgui.BulletText(summary)
		}
		if view.HiddenConflictCount != 0 {
			imgui.TextDisabled(fmt.Sprintf("%d additional conflicts hidden", view.HiddenConflictCount))
		}
	}

	imgui.Separator()
	w.Disabled(!view.CanLeave, w.Button("Leave Session", panel.app.DoLeaveCollaborationSession)).Build()
}
