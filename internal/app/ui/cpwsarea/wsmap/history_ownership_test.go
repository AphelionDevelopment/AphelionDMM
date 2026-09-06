package wsmap

import (
	"context"
	"path/filepath"
	"testing"

	"sdmm/internal/util"
)

func secondHistoryWorkspace(t *testing.T, first *WsMap, app *selectionTestApp) *WsMap {
	t.Helper()
	m := first.Map().Dmm().Copy()
	m.Name = "other.dmm"
	m.Path.Absolute = filepath.Join(t.TempDir(), m.Name)
	second := New(app, &m)
	t.Cleanup(second.Map().Editor().Close)
	first.OnFocusChange(false)
	second.OnFocusChange(true)
	t.Cleanup(func() { second.OnFocusChange(false) })
	app.commands.SetStack(second.CommandStackId())
	return second
}

func TestHistoryAcknowledgementAfterTabSwitchKeepsMapOwner(t *testing.T) {
	first, app := newSelectionWorkspace(t)
	grab := activateSelectionWorkspace(t, first)
	network, transport, document := selectionNetwork(t, first)
	initialHash := resizeHash(t, document.Snapshot())
	if err := grab.Nudge(util.Point{X: 1}); err != nil {
		t.Fatal(err)
	}
	operation := transport.next(t)
	second := secondHistoryWorkspace(t, first, app)
	secondBefore := resizeSnapshot(t, second.Map().Editor())
	acceptSelection(t, network, document, operation)
	runSelectionJob(t, app)
	first.Map().Editor().ProcessCollaborationUpdates()
	if !app.commands.HasUndoV(first.CommandStackId()) || app.commands.HasUndoV(second.CommandStackId()) {
		t.Fatal("accepted map edit entered the active tab's history instead of its source map")
	}
	if !app.commands.UndoAsyncV(first.CommandStackId(), nil) {
		t.Fatal("source map undo unavailable")
	}
	acceptSelection(t, network, document, transport.next(t))
	runSelectionJob(t, app)
	first.Map().Editor().ProcessCollaborationUpdates()
	if resizeHash(t, resizeSnapshot(t, first.Map().Editor())) != initialHash || resizeHash(t, resizeSnapshot(t, second.Map().Editor())) != resizeHash(t, secondBefore) {
		t.Fatal("source-map undo changed the wrong document")
	}
	if _, err := second.Map().Editor().SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryInactiveLocalResizeKeepsMapOwner(t *testing.T) {
	first, app := newSelectionWorkspace(t)
	second := secondHistoryWorkspace(t, first, app)
	initial := resizeSnapshot(t, first.Map().Editor())
	if err := first.Map().Editor().ResizeMap(3, 3, 1); err != nil {
		t.Fatal(err)
	}
	if !app.commands.HasUndoV(first.CommandStackId()) || app.commands.HasUndoV(second.CommandStackId()) {
		t.Fatal("inactive local resize entered another map's history")
	}
	app.commands.UndoV(first.CommandStackId())
	if resizeHash(t, resizeSnapshot(t, first.Map().Editor())) != resizeHash(t, initial) {
		t.Fatal("inactive resize undo lost map state")
	}
}
