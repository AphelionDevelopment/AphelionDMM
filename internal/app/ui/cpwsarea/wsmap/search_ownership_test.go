package wsmap

import (
	"context"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
	"sdmm/internal/app/ui/cpsearch"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func TestSearchRefreshAfterResizeUndoAndSnapshot(t *testing.T) {
	ws, app := newSelectionWorkspace(t)
	activateSelectionWorkspace(t, ws)
	e := ws.Map().Editor()
	var search cpsearch.Search
	search.Init(&searchWorkspaceApp{current: e})
	search.SetFocused(true)
	t.Cleanup(func() { search.SetFocused(false); search.Free() })
	search.SearchByPath("/obj/foo")
	check := func(stage string) {
		t.Helper()
		before := resizeHash(t, resizeSnapshot(t, e))
		e.SetFlickInstance(nil)
		pressSelectionShortcut(glfw.KeyF3)
		flicks := e.FlickInstance()
		if len(flicks) != 1 {
			t.Errorf("%s: expected one current result highlight, got %d", stage, len(flicks))
			return
		}
		instance := flicks[0].Instance
		found := false
		for _, tile := range e.Dmm().Tiles {
			for _, current := range tile.Instances() {
				found = found || current == instance
			}
		}
		if !found {
			t.Errorf("%s: F3 highlighted a detached instance", stage)
		}
		if resizeHash(t, resizeSnapshot(t, e)) != before {
			t.Errorf("%s: navigation changed map authority", stage)
		}
	}
	resizeWorkspace(t, e, 1, 1, 1)
	check("shrink")
	app.commands.UndoV(e.Dmm().Path.Absolute)
	check("undo resize")
	app.commands.RedoV(e.Dmm().Path.Absolute)
	check("redo resize")
	if err := e.RefreshCollaborationSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	check("snapshot refresh")
}

func TestSearchBatchNetworkOutcomeAndUndo(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		name := "rejected"
		if accepted {
			name = "accepted"
		}
		t.Run(name, func(t *testing.T) {
			ws, app := newSelectionWorkspace(t)
			activateSelectionWorkspace(t, ws)
			network, transport, document := selectionNetwork(t, ws)
			e := ws.Map().Editor()
			initial := document.Snapshot()
			initialHash := resizeHash(t, initial)
			e.CommitInstanceBatch([]*dmminstance.Instance{e.Dmm().Tiles[0].Instances()[2], e.Dmm().Tiles[1].Instances()[2]}, nil, "Delete search results")
			operation := transport.next(t)
			if len(operation.Changes) != 2 {
				t.Fatal("search batch was not submitted as one two-tile operation")
			}
			if _, err := e.SaveSnapshot(context.Background()); err == nil || app.commands.HasUndoV(e.Dmm().Path.Absolute) {
				t.Fatal("search submission became durable/history before acknowledgement")
			}
			if accepted {
				acceptSelection(t, network, document, operation)
			} else {
				receiveSelection(t, network, protocol.ServerOperationRejected, protocol.OperationRejectedPayload{OperationID: operation.OperationID, Code: "precondition_failed", Message: "search verification conflict", Revision: initial.Revision, MapHash: initialHash})
			}
			runSelectionJob(t, app)
			e.ProcessCollaborationUpdates()
			if accepted {
				if len(e.Dmm().Tiles[0].Instances()) != 2 || !app.commands.HasUndoV(e.Dmm().Path.Absolute) {
					t.Fatal("accepted search deletion did not update display/history")
				}
				app.commands.UndoV(e.Dmm().Path.Absolute)
				acceptSelection(t, network, document, transport.next(t))
				runSelectionJob(t, app)
				e.ProcessCollaborationUpdates()
			} else if app.commands.HasUndoV(e.Dmm().Path.Absolute) || len(app.errors) != 1 || len(network.Conflicts()) != 1 {
				t.Fatal("rejected search batch lost conflict state or entered history")
			}
			if resizeHash(t, resizeSnapshot(t, e)) != initialHash {
				t.Fatal("search rejection/undo did not restore exact authority")
			}
		})
	}
}

func TestSearchBatchRejectsDetachedTargetBeforeMutation(t *testing.T) {
	ws, app := newSelectionWorkspace(t)
	activateSelectionWorkspace(t, ws)
	e := ws.Map().Editor()
	detached := e.Dmm().Tiles[1].Instances()[2]
	if err := e.RefreshCollaborationSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := resizeHash(t, resizeSnapshot(t, e))
	first := e.Dmm().Tiles[0].Instances()[2]
	e.CommitInstanceBatch([]*dmminstance.Instance{first, detached}, nil, "Delete stale search results")
	if resizeHash(t, resizeSnapshot(t, e)) != before || e.Dmm().Tiles[0].Instances()[2] != first || !e.CanStartMapEdit() {
		t.Fatal("stale batch changed display/authority or retained an earlier capture")
	}
	if len(app.errors) != 1 || app.commands.HasUndoV(e.Dmm().Path.Absolute) {
		t.Fatal("stale batch was not reported without committing history")
	}
}

func TestSearchRefreshAfterLocalAndRemoteChanges(t *testing.T) {
	ws, _ := newSelectionWorkspace(t)
	activateSelectionWorkspace(t, ws)
	e := ws.Map().Editor()
	var search cpsearch.Search
	search.Init(&searchWorkspaceApp{current: e})
	search.SetFocused(true)
	t.Cleanup(func() { search.SetFocused(false); search.Free() })
	search.SearchByPath("/obj/foo")
	check := func(stage string, expected int) {
		t.Helper()
		current := make(map[string]bool)
		for _, tile := range e.Dmm().Tiles {
			for _, instance := range tile.Instances() {
				if instance.Prefab().Path() == "/obj/foo" {
					current[instance.StableID()] = true
				}
			}
		}
		if len(current) != expected {
			t.Fatalf("%s: fixture has %d matches, expected %d", stage, len(current), expected)
		}
		before := resizeHash(t, resizeSnapshot(t, e))
		for range expected {
			e.SetFlickInstance(nil)
			pressSelectionShortcut(glfw.KeyF3)
			flicks := e.FlickInstance()
			if len(flicks) != 1 || !current[flicks[0].Instance.StableID()] {
				t.Fatalf("%s: missing, duplicate or removed search result", stage)
			}
			instance := flicks[0].Instance
			found := false
			for _, candidate := range e.Dmm().GetTile(instance.Coord()).Instances() {
				found = found || candidate == instance
			}
			if !found {
				t.Fatalf("%s: search returned an old display instance", stage)
			}
			delete(current, instance.StableID())
		}
		if resizeHash(t, resizeSnapshot(t, e)) != before {
			t.Fatalf("%s: search changed authority", stage)
		}
	}
	i := e.Dmm().Tiles[0].Instances()[2]
	e.InstanceReplace(i, dmmprefab.New(0, i.Prefab().Path(), dmvars.Set(i.Prefab().Vars(), "dir", "4")))
	e.CommitOperation("Search ownership local edit")
	check("local edit", 32)
	network, _, document := selectionNetwork(t, ws)
	check("network attachment", 32)
	initial := document.Snapshot()
	tile := initial.Tiles[0]
	after := model.CloneTileState(tile.State)
	after.Prefabs = after.Prefabs[:2]
	id, err := model.NewOperationID()
	if err != nil {
		t.Fatal(err)
	}
	actor, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	acceptSelection(t, network, document, model.Operation{
		ProtocolVersion: model.ProtocolVersion, DocumentID: initial.DocumentID, ActorID: actor, OperationID: id,
		BaseRevision: initial.Revision, EnvironmentHash: initial.EnvironmentHash, BaseMapHash: resizeHash(t, initial),
		Kind: model.OperationKindTileChange, Changes: []model.TileChange{{Coord: tile.Coord, Before: tile.State, After: after}},
	})
	e.ProcessCollaborationUpdates()
	check("remote removal", 31)
}

func TestSearchWaitsForGestureAndRecoversOnCancel(t *testing.T) {
	ws, _ := newSelectionWorkspace(t)
	activateSelectionWorkspace(t, ws)
	e := ws.Map().Editor()
	before := resizeHash(t, resizeSnapshot(t, e))
	var search cpsearch.Search
	search.Init(&searchWorkspaceApp{current: e})
	search.SetFocused(true)
	t.Cleanup(func() { search.SetFocused(false); search.Free() })
	search.SearchByPath("/obj/foo")
	move, err := e.BeginSelectionMove(util.Bounds{X1: 1, Y1: 1, X2: 1, Y2: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.PreviewSelectionMove(move, util.Point{X: 1}); err != nil {
		t.Fatal(err)
	}
	for _, queryDuringGesture := range []bool{false, true} {
		if queryDuringGesture {
			search.SearchByPath("/obj/foo")
		}
		e.SetFlickInstance(nil)
		pressSelectionShortcut(glfw.KeyF3)
		if len(e.FlickInstance()) != 0 {
			t.Error("search navigated a result while a gesture owned the display")
		}
	}
	e.FinishSelectionMove(move, true)
	e.SetFlickInstance(nil)
	pressSelectionShortcut(glfw.KeyF3)
	flicks := e.FlickInstance()
	if len(flicks) != 1 || flicks[0].Instance != e.Dmm().Tiles[0].Instances()[2] {
		t.Error("cancel did not resume search with restored current instances")
	}
	if resizeHash(t, resizeSnapshot(t, e)) != before {
		t.Error("search interfered with exact gesture cancellation")
	}
}

func TestSearchInvalidReplacementDoesNotSubmitOrBlockRetry(t *testing.T) {
	ws, app := newSelectionWorkspace(t)
	e := ws.Map().Editor()
	network, transport, document := selectionNetwork(t, ws)
	before := resizeHash(t, resizeSnapshot(t, e))
	first := e.Dmm().Tiles[0].Instances()[2]
	second := e.Dmm().Tiles[1].Instances()[2]
	targets := []*dmminstance.Instance{first, second}
	e.CommitInstanceBatch(targets, dmmprefab.New(dmmprefab.IdStage, "/obj/foo", nil), "Invalid replacement")
	if len(transport.sent) != 0 || len(app.errors) != 1 || !e.CanStartMapEdit() || app.commands.HasUndoV(e.Dmm().Path.Absolute) {
		t.Fatal("invalid replacement submitted or left blocked editing/history")
	}
	if resizeHash(t, resizeSnapshot(t, e)) != before || e.Dmm().Tiles[0].Instances()[2] != first || e.Dmm().Tiles[1].Instances()[2] != second {
		t.Fatal("invalid replacement changed authority or display ownership")
	}
	replacement := dmmprefab.New(dmmprefab.IdNone, "/obj/foo", dmvars.Set(first.Prefab().Vars(), "dir", "4"))
	e.CommitInstanceBatch(targets, replacement, "Corrected replacement")
	operation := transport.next(t)
	if len(operation.Changes) != 2 {
		t.Fatal("corrected retry did not submit one two-tile action")
	}
	acceptSelection(t, network, document, operation)
	runSelectionJob(t, app)
	e.ProcessCollaborationUpdates()
	if resizeSnapshot(t, e).Revision != 1 || len(app.errors) != 1 || !app.commands.HasUndoV(e.Dmm().Path.Absolute) {
		t.Fatal("corrected retry was not acknowledged once")
	}
	app.commands.UndoV(e.Dmm().Path.Absolute)
	acceptSelection(t, network, document, transport.next(t))
	runSelectionJob(t, app)
	e.ProcessCollaborationUpdates()
	if resizeHash(t, resizeSnapshot(t, e)) != before {
		t.Fatal("corrected replacement inverse did not restore the map")
	}
}
