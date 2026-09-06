package cpwsarea

import (
	"sdmm/internal/app/ui/cpwsarea/workspace"
	"testing"
)

func TestSaveBeforeClosePropagatesFailure(t *testing.T) {
	first, second := &saveCloseContent{result: true}, &saveCloseContent{}
	called, closed := false, true
	if saveWorkspacesBeforeClose([]*workspace.Workspace{workspace.New(first), workspace.New(second)}, func(result bool) { called, closed = true, result }) {
		t.Fatal("failed save allowed workspace disposal")
	}
	if first.saves != 1 || second.saves != 1 || !called || closed {
		t.Fatalf("save/close results: %d, %d, callback=%t closed=%t", first.saves, second.saves, called, closed)
	}
	second.result = true
	if !saveWorkspacesBeforeClose([]*workspace.Workspace{workspace.New(second)}, nil) {
		t.Fatal("successful save blocked close")
	}
}

type saveCloseContent struct {
	guardTestContent
	result bool
	saves  int
}

func (content *saveCloseContent) Save() bool { content.saves++; return content.result }

func TestWorkspaceDirtyStateIncludesAuthority(t *testing.T) {
	area := &WsArea{app: &guardTestApp{}}
	if !area.isWorkspaceUnsaved(workspace.New(&dirtyCloseContent{})) {
		t.Fatal("authoritative changes omitted from close check")
	}
}

type dirtyCloseContent struct{ guardTestContent }

func (*dirtyCloseContent) HasUnsavedChanges() bool { return true }
