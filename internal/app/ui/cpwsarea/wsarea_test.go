package cpwsarea

import (
	"testing"

	"sdmm/internal/app/command"
	"sdmm/internal/app/ui/cpwsarea/workspace"
)

func TestCloseWorkspacesGuardRunsBeforeDisposal(t *testing.T) {
	commands := command.NewStorage()
	application := &guardTestApp{commands: commands}
	content := &guardTestContent{}
	ws := workspace.New(content)
	area := &WsArea{app: application, workspaces: []*workspace.Workspace{ws}}
	callbackResult := true

	area.closeWorkspacesGentlyV([]*workspace.Workspace{ws}, func() bool { return false }, func(closed bool) {
		callbackResult = closed
	})

	if content.disposed || len(area.workspaces) != 1 || callbackResult {
		t.Fatalf("guarded close state: disposed=%t workspaces=%d callback=%t", content.disposed, len(area.workspaces), callbackResult)
	}
}

func TestCloseWorkspaceGuardRunsBeforeDisposal(t *testing.T) {
	commands := command.NewStorage()
	application := &guardTestApp{commands: commands}
	content := &guardTestContent{}
	ws := workspace.New(content)
	area := &WsArea{app: application, workspaces: []*workspace.Workspace{ws}}
	callbackResult := true

	area.closeWorkspaceGentlyV(ws, func() bool { return false }, func(closed bool) {
		callbackResult = closed
	})

	if content.disposed || len(area.workspaces) != 1 || callbackResult {
		t.Fatalf("guarded close state: disposed=%t workspaces=%d callback=%t", content.disposed, len(area.workspaces), callbackResult)
	}
}

type guardTestApp struct {
	App
	commands *command.Storage
}

func (app *guardTestApp) CommandStorage() *command.Storage {
	return app.commands
}

type guardTestContent struct {
	workspace.Content
	disposed bool
}

func (*guardTestContent) Name() string  { return "guard test" }
func (*guardTestContent) Title() string { return "Guard Test" }

func (content *guardTestContent) Dispose() {
	content.disposed = true
}
