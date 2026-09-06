package editor

import (
	"context"
	"testing"

	"sdmm/internal/aphelion/collab/executor"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
)

func TestCommitCannotFallBackAfterCaptureFailure(t *testing.T) {
	e := selectionEditor(t)
	app := &noopReportingApp{editorTestApp: e.app.(*editorTestApp)}
	e.app = app
	authority := e.executor
	before, err := authority.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	instance := e.dmm.Tiles[0].Instances()[2]
	instance.SetStableID("invalid-capture-identity")
	prefab := instance.Prefab()
	e.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, prefab.Path(), dmvars.Set(prefab.Vars(), "dir", "4")))
	if e.collaborationErr == nil {
		t.Fatal("fixture did not fail tile capture")
	}
	e.CommitOperation("Invalid capture")
	assertNoFallbackCommit(t, e, app)
	// A deliberately installed, validated attachment restores authority and
	// clears the capture fault; it does not import the unvalidated display edit.
	if err := e.AttachCollaborationExecutor(authority); err != nil {
		t.Fatal(err)
	}
	recovered, err := e.SaveSnapshot(context.Background())
	if err != nil {
		t.Fatalf("valid attachment did not recover editing: %v", err)
	}
	want, _ := before.Hash()
	got, _ := recovered.Hash()
	if got != want || recovered.Revision != 0 {
		t.Fatal("recovery adopted the unvalidated display edit")
	}
}

func TestCommitCannotFallBackWithoutExecutor(t *testing.T) {
	e := selectionEditor(t)
	app := &noopReportingApp{editorTestApp: e.app.(*editorTestApp)}
	e.app = app
	e.Close()
	instance := e.dmm.Tiles[0].Instances()[2]
	prefab := instance.Prefab()
	e.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, prefab.Path(), dmvars.Set(prefab.Vars(), "dir", "4")))
	e.CommitOperation("Late edit after close")
	assertNoFallbackCommit(t, e, app)
}

func TestCommitRequiresLiveMapHistory(t *testing.T) {
	for _, invalidation := range []string{"dispose", "free"} {
		t.Run(invalidation, func(t *testing.T) {
			e := selectionEditor(t)
			app := &noopReportingApp{editorTestApp: e.app.(*editorTestApp)}
			e.app = app
			if invalidation == "dispose" {
				app.commands.DisposeStack("test")
			} else {
				app.commands.Free()
			}
			app.commands.SetStack("test") // A new lifetime must not adopt old edits.
			if e.CanChangeMapSize() || e.ResizeMap(2, 1, 1) == nil {
				t.Fatal("disposed map history allowed resize")
			}
			instance := e.dmm.Tiles[0].Instances()[2]
			prefab := instance.Prefab()
			e.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, prefab.Path(), dmvars.Set(prefab.Vars(), "dir", "4")))
			e.CommitOperation("Edit after history disposal")
			assertNoFallbackCommit(t, e, app)
		})
	}
}

func assertNoFallbackCommit(t *testing.T, e *Editor, app *noopReportingApp) {
	t.Helper()
	if app.commands.HasUndoV("test") {
		t.Fatal("failed authority created a legacy undo command")
	}
	if len(app.errors) != 1 {
		t.Fatalf("failed authority reported %d errors, want one", len(app.errors))
	}
	if got := e.pMap.Snapshot().Initial().Tiles[0].Instances()[2].Prefab().Vars().ValueV("dir", ""); got != "2" {
		t.Fatal("failed authority advanced the legacy snapshot")
	}
	if _, err := e.SaveSnapshot(context.Background()); err == nil {
		t.Fatal("failed authority allowed Save")
	}
	if e.executor != nil {
		assertAuthorityRevision(t, e.executor)
	}
}

func assertAuthorityRevision(t *testing.T, execution executor.Executor) {
	t.Helper()
	snapshot, err := execution.Snapshot(context.Background())
	if err != nil || snapshot.Revision != 0 {
		t.Fatal("failed capture changed authoritative revision")
	}
}
