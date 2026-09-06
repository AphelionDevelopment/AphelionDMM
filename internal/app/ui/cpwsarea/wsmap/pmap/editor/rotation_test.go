package editor

import (
	"context"
	"testing"

	"sdmm/internal/app/command"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmsnap"
	"sdmm/internal/util"
)

type rotationLevelMap struct {
	*editorTestAttachedMap
	level int
}

func (m *rotationLevelMap) ActiveLevel() int { return m.level }

func TestEditorRotationRefusesSelectionOnAnotherLevel(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)
	environment := editorTestEnvironment()
	m := editorTestMap(environment)
	app := &editorTestApp{commands: command.NewStorage(), environment: environment, paths: dm.NewPathsFilterEmpty()}
	app.commands.SetStack("test")
	e := New(app, &rotationLevelMap{editorTestAttachedMap: &editorTestAttachedMap{snapshot: dmmsnap.New(m)}, level: 2}, m)
	if _, err := e.RotateSelection(util.Bounds{X1: 1, Y1: 1, X2: 1, Y2: 1}, 1, true); err == nil {
		t.Fatal("rotated a selection on a non-visible level")
	}
	assertEditorDirection(t, m, "2")
	if app.commands.HasUndoV("test") {
		t.Fatal("refused rotation entered history")
	}
}

func TestEditorRotationUsesOperationUndoAndNetworkAcknowledgement(t *testing.T) {
	for _, network := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "network"}[network], func(t *testing.T) {
			dmmap.PrefabStorage.Free()
			t.Cleanup(dmmap.PrefabStorage.Free)
			environment := editorTestEnvironment()
			m := editorTestMap(environment)
			app := &editorTestApp{commands: command.NewStorage(), environment: environment, paths: dm.NewPathsFilterEmpty(), runLater: make(chan func(), 8)}
			app.commands.SetStack("test")
			e := New(app, &editorTestAttachedMap{snapshot: dmmsnap.New(m)}, m)
			before, err := e.CollaborationSnapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var remote *deferredAsyncExecutor
			if network {
				remote = newDeferredAsyncExecutor(t, before, e.actorID)
				if err := e.AttachCollaborationExecutor(remote); err != nil {
					t.Fatal(err)
				}
			}
			id := m.Tiles[0].Instances()[2].StableID()
			if _, err := e.RotateSelection(util.Bounds{X1: 1, Y1: 1, X2: 1, Y2: 1}, 1, true); err != nil {
				t.Fatal(err)
			}
			if network {
				if app.commands.HasUndoV("test") {
					t.Fatal("speculative rotation created undo")
				}
				accepted, err := remote.Execute(context.Background(), remote.operation)
				if err != nil {
					t.Fatal(err)
				}
				remote.complete(accepted, nil)
				app.runScheduled(t)
			}
			assertEditorDirection(t, m, "8")
			if m.Tiles[0].Instances()[2].StableID() != id {
				t.Fatal("rotation replaced instance identity")
			}
			if !app.commands.HasUndoV("test") {
				t.Fatal("rotation did not create undo")
			}
			app.commands.UndoV("test")
			if network {
				remote.resolve(t)
				app.runScheduled(t)
			}
			assertEditorDirection(t, m, "2")
			app.commands.RedoV("test")
			if network {
				remote.resolve(t)
				app.runScheduled(t)
			}
			assertEditorDirection(t, m, "8")
		})
	}
}
