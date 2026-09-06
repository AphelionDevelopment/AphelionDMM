package wsmap

import (
	"context"
	"testing"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/executor"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/editor"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func resizeWorkspace(t *testing.T, e *editor.Editor, x, y, z int) {
	t.Helper()
	if err := e.ResizeMap(x, y, z); err != nil {
		t.Fatal(err)
	}
}

func resizeSnapshot(t *testing.T, e *editor.Editor) model.Snapshot {
	t.Helper()
	snapshot, err := e.SaveSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func resizeHash(t *testing.T, snapshot model.Snapshot) string {
	t.Helper()
	hash, err := snapshot.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestResizeUndoKeepsEarlierEditHistoryUsable(t *testing.T) {
	ws, app := newSelectionWorkspace(t)
	e := ws.Map().Editor()
	initial := resizeSnapshot(t, e)
	instance := e.Dmm().Tiles[0].Instances()[2]
	prefab := instance.Prefab()
	e.InstanceReplace(instance, dmmprefab.New(0, prefab.Path(), dmvars.Set(prefab.Vars(), "dir", "4")))
	e.CommitOperation("Edit before resize")
	resizeWorkspace(t, e, 3, 3, 1)
	app.commands.UndoV(e.Dmm().Path.Absolute)
	var undoErr error
	if !app.commands.UndoAsyncV(e.Dmm().Path.Absolute, func(err error) { undoErr = err }) {
		t.Fatal("earlier edit was removed from history")
	}
	if undoErr != nil {
		t.Fatalf("resize broke earlier edit undo: %v", undoErr)
	}
	if resizeHash(t, resizeSnapshot(t, e)) != resizeHash(t, initial) {
		t.Fatal("resize/earlier-edit undo did not restore the initial map")
	}
}

func TestResizeRedoRetainsExpandedTileIdentities(t *testing.T) {
	ws, app := newSelectionWorkspace(t)
	e := ws.Map().Editor()
	resizeWorkspace(t, e, 5, 4, 2)
	expanded := resizeSnapshot(t, e)
	app.commands.UndoV(e.Dmm().Path.Absolute)
	app.commands.RedoV(e.Dmm().Path.Absolute)
	if resizeHash(t, resizeSnapshot(t, e)) != resizeHash(t, expanded) {
		t.Fatal("resize redo regenerated stable IDs or changed map contents")
	}
	selectionNetwork(t, ws)
	var undoErr error
	app.commands.UndoAsyncV(e.Dmm().Path.Absolute, func(err error) { undoErr = err })
	if undoErr == nil || resizeHash(t, resizeSnapshot(t, e)) != resizeHash(t, expanded) {
		t.Fatal("old resize history crossed a later attachment")
	}
}

func TestResizeFailureRetainsDimensionsAuthorityAndHistory(t *testing.T) {
	ws, app := newSelectionWorkspace(t)
	e := ws.Map().Editor()
	initial := resizeSnapshot(t, e)
	app.environment = nil // The environment became unavailable before Set.
	if err := e.ResizeMap(3, 3, 1); err == nil {
		t.Fatal("missing environment accepted")
	}
	if e.Dmm().MaxX != initial.MaxX || e.Dmm().MaxY != initial.MaxY || e.Dmm().MaxZ != initial.MaxZ {
		t.Fatal("failed resize published new dimensions")
	}
	if app.commands.HasUndoV(e.Dmm().Path.Absolute) {
		t.Fatal("failed resize entered undo history")
	}
}

func TestResizeUndoRejectsModifiedInactiveCheckpoint(t *testing.T) {
	ws, app := newSelectionWorkspace(t)
	e := ws.Map().Editor()
	initial := resizeSnapshot(t, e)
	document, err := engine.NewDocument(initial)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	local, err := executor.NewLocal(document, actor)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.AttachCollaborationExecutor(local); err != nil {
		t.Fatal(err)
	}
	resizeWorkspace(t, e, 3, 3, 1)
	current := resizeSnapshot(t, e)
	// A retained executor is deliberately changed outside the UI history.
	before := initial.Tiles[0].State
	after := model.CloneTileState(before)
	after.Prefabs[2].Vars["dir"] = "4"
	operationID, err := model.NewOperationID()
	if err != nil {
		t.Fatal(err)
	}
	_, err = local.Execute(context.Background(), model.Operation{ProtocolVersion: model.ProtocolVersion, DocumentID: initial.DocumentID, ActorID: actor, OperationID: operationID, EnvironmentHash: initial.EnvironmentHash, BaseRevision: initial.Revision, BaseMapHash: resizeHash(t, initial), Kind: model.OperationKindTileChange, Changes: []model.TileChange{{Coord: initial.Tiles[0].Coord, Before: before, After: after}}})
	if err != nil {
		t.Fatal(err)
	}
	var undoErr error
	app.commands.UndoAsyncV(e.Dmm().Path.Absolute, func(err error) { undoErr = err })
	if undoErr == nil || resizeHash(t, resizeSnapshot(t, e)) != resizeHash(t, current) {
		t.Fatal("resize undo installed an unexpectedly changed checkpoint")
	}
	if !app.commands.HasUndoV(e.Dmm().Path.Absolute) {
		t.Fatal("failed resize undo moved the history pointer")
	}
}

func TestResizeEditChainsAndFailedUndoAreRecoverable(t *testing.T) {
	ws, app := newSelectionWorkspace(t)
	grab := activateSelectionWorkspace(t, ws)
	e := ws.Map().Editor()
	states := []model.Snapshot{resizeSnapshot(t, e)}
	edit := func(dir string) {
		i := e.Dmm().Tiles[0].Instances()[2]
		p := i.Prefab()
		e.InstanceReplace(i, dmmprefab.New(0, p.Path(), dmvars.Set(p.Vars(), "dir", dir)))
		e.CommitOperation("Resize chain edit")
		states = append(states, resizeSnapshot(t, e))
	}
	edit("4")
	ws.Map().SetActiveLevel(2)
	resizeWorkspace(t, e, 3, 3, 1)
	states = append(states, resizeSnapshot(t, e))
	if states[2].DocumentID == states[1].DocumentID || ws.Map().ActiveLevel() != 1 || grab.HasSelectedArea() {
		t.Fatal("resize retained the document identity, removed level or stale selection")
	}
	edit("8")
	resizeWorkspace(t, e, 5, 4, 2)
	states = append(states, resizeSnapshot(t, e))
	edit("1")
	for index := len(states) - 2; index >= 0; index-- {
		if index == 3 { // Undoing the second resize: failed environment read is retryable.
			environment := app.environment
			app.environment = nil
			var undoErr error
			app.commands.UndoAsyncV(e.Dmm().Path.Absolute, func(err error) { undoErr = err })
			app.environment = environment
			if undoErr == nil || resizeHash(t, resizeSnapshot(t, e)) != resizeHash(t, states[index+1]) {
				t.Fatal("failed undo changed the displayed map")
			}
		}
		var undoErr error
		app.commands.UndoAsyncV(e.Dmm().Path.Absolute, func(err error) { undoErr = err })
		if undoErr != nil || resizeHash(t, resizeSnapshot(t, e)) != resizeHash(t, states[index]) {
			t.Fatalf("undo chain at %d: %v", index, undoErr)
		}
	}
	for index := 1; index < len(states); index++ {
		var redoErr error
		app.commands.RedoAsyncV(e.Dmm().Path.Absolute, func(err error) { redoErr = err })
		if redoErr != nil || resizeHash(t, resizeSnapshot(t, e)) != resizeHash(t, states[index]) {
			t.Fatalf("redo chain at %d: %v", index, redoErr)
		}
	}
	// A new attachment invalidates both maintenance and normal edit commands.
	selectionNetwork(t, ws)
	var undoErr error
	app.commands.UndoAsyncV(e.Dmm().Path.Absolute, func(err error) { undoErr = err })
	if undoErr == nil {
		t.Fatal("old edit history crossed an attachment change")
	}
	if e.CanChangeMapSize() {
		t.Fatal("network attachment enabled resize")
	}
	if _, err := e.BeginSelectionMove(util.Bounds{X1: 1, Y1: 1, X2: 1, Y2: 1}, 1); err != nil {
		t.Fatal(err)
	}
	if err := e.ResizeMap(2, 2, 1); err == nil {
		t.Fatal("resize accepted an open gesture/network map")
	}
}
