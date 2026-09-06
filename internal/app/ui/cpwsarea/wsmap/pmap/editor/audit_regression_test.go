package editor

import (
	"context"
	"testing"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/app/command"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmsnap"
	"sdmm/internal/dmapi/dmvars"
)

func TestAuditAcknowledgementPreservesActiveGesture(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)
	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{commands: command.NewStorage(), environment: environment, paths: dm.NewPathsFilterEmpty(), runLater: make(chan func(), 8)}
	application.commands.SetStack("test")
	editor := New(application, &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}, mapState)
	if editor.collaborationErr != nil {
		t.Fatal(editor.collaborationErr)
	}
	deferred := newDeferredAsyncExecutor(t, editor.authoritative, editor.actorID)
	editor.executor = deferred
	instance := mapState.Tiles[0].Instances()[2]
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), dmvars.Set(instance.Prefab().Vars(), "dir", "4")))
	editor.CommitOperation("First Network Change")
	instance = mapState.Tiles[0].Instances()[2]
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), dmvars.Set(instance.Prefab().Vars(), "dir", "8")))
	coord := model.Coord{X: 1, Y: 1, Z: 1}
	if _, exists := editor.pendingChanges[coord]; !exists {
		t.Fatal("second gesture was not captured")
	}
	// First edit finishes while the next gesture remains open, before its mouse-up commit.
	deferred.resolve(t)
	application.runScheduled(t)
	if _, exists := editor.pendingChanges[coord]; !exists {
		t.Fatal("acknowledgement of first edit erased active second gesture; its commit now returns without submitting")
	}
	assertEditorDirection(t, mapState, "8")
	editor.CommitOperation("Second Network Change")
	deferred.resolve(t)
	application.runScheduled(t)
	if editor.authoritative.Revision != 2 {
		t.Fatalf("second gesture was not acknowledged: revision %d", editor.authoritative.Revision)
	}
	assertEditorDirection(t, mapState, "8")
}

func TestAuditOldAttachmentCompletionCannotRestoreExecutor(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)
	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{commands: command.NewStorage(), environment: environment, paths: dm.NewPathsFilterEmpty(), runLater: make(chan func(), 8)}
	application.commands.SetStack("test")
	editor := New(application, &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}, mapState)
	deferred := newDeferredAsyncExecutor(t, editor.authoritative, editor.actorID)
	editor.executor = deferred
	instance := mapState.Tiles[0].Instances()[2]
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), dmvars.Set(instance.Prefab().Vars(), "dir", "4")))
	editor.CommitOperation("Old Attachment")
	deferred.resolve(t)
	replacement := newDeferredAsyncExecutor(t, editor.authoritative, editor.actorID)
	if err := editor.AttachCollaborationExecutor(replacement); err != nil {
		t.Fatal(err)
	}
	application.runScheduled(t)
	if editor.executor != replacement {
		t.Fatal("old completion restored the obsolete executor")
	}
	snapshot, err := editor.CollaborationSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 0 {
		t.Fatalf("old completion advanced replacement to %d", snapshot.Revision)
	}
	assertEditorDirection(t, mapState, "2")
}
