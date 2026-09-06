package editor

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"sdmm/internal/aphelion/collab/executor"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

type countedSnapshotExecutor struct {
	executor.Executor
	snapshots int
	failFirst bool
}

func (e *countedSnapshotExecutor) Snapshot(ctx context.Context) (model.Snapshot, error) {
	e.snapshots++
	if e.failFirst && e.snapshots == 1 {
		return model.Snapshot{}, errors.New("controlled snapshot failure")
	}
	return e.Executor.Snapshot(ctx)
}

type noopReportingApp struct {
	*editorTestApp
	errors []error
}

func (app *noopReportingApp) ReportCollaborationError(_ string, err error) {
	app.errors = append(app.errors, err)
}

func TestChangedGestureRetainsCaptureWhenSnapshotFails(t *testing.T) {
	e := selectionEditor(t)
	app := &noopReportingApp{editorTestApp: e.app.(*editorTestApp)}
	e.app = app
	counted := &countedSnapshotExecutor{Executor: e.executor, failFirst: true}
	e.executor = counted
	coord := util.Point{X: 1, Y: 1, Z: 1}
	e.BeginTileChange(coord)
	instance := e.dmm.GetTile(coord).Instances()[2]
	before := instance.Prefab()
	instance.SetPrefab(dmmprefab.New(dmmprefab.IdNone, before.Path(), dmvars.Set(before.Vars(), "dir", "4")))
	e.CommitOperation("Snapshot failure")
	if len(e.pendingChanges) != 1 || len(app.errors) != 1 || e.app.CommandStorage().HasUndoV("test") {
		t.Fatal("snapshot failure discarded the captured edit, missed the error, or created undo")
	}
	if _, err := e.SaveSnapshot(context.Background()); err == nil {
		t.Fatal("failed commit left the unfinished edit saveable")
	}
	// The same capture remains retryable after the transient read failure.
	e.CommitOperation("Retry captured edit")
	if len(e.pendingChanges) != 0 || !e.app.CommandStorage().HasUndoV("test") {
		t.Fatal("retry did not submit the retained before/after edit")
	}
}

func BenchmarkUnchangedGestureSnapshotOrdering(b *testing.B) {
	for _, cells := range []int{100, 1000, 10000} {
		for _, readFirst := range []bool{true, false} {
			name := "defer_until_changed"
			if readFirst {
				name = "read_first_control"
			}
			b.Run(fmt.Sprintf("cells=%d/%s", cells, name), func(b *testing.B) {
				e := selectionEditor(b)
				prefabs := e.dmm.Tiles[0].Instances().Prefabs()
				for index := 1; index < cells; index++ {
					tile := &dmmap.Tile{Coord: util.Point{X: index%100 + 1, Y: index/100 + 1, Z: 1}}
					tile.InstancesSet(prefabs)
					e.dmm.Tiles = append(e.dmm.Tiles, tile)
				}
				e.dmm.MaxX, e.dmm.MaxY = 100, cells/100
				for tileIndex, tile := range e.dmm.Tiles {
					for prefabIndex, instance := range tile.Instances() {
						instance.SetStableID(fmt.Sprintf("01890f3e-7b5c-7abc-8def-%012x", tileIndex*3+prefabIndex+1))
					}
				}
				e.documentID = "01890f3e-7b5c-7abc-8def-111111111111"
				e.initializeCollaboration()
				if e.collaborationErr != nil {
					b.Fatal(e.collaborationErr)
				}
				want, err := e.authoritative.Hash()
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					e.BeginTileChange(util.Point{X: 1, Y: 1, Z: 1})
					if readFirst {
						// Reproduce the prior successful no-op path: the full read
						// precedes the same touched-tile comparison and empty commit.
						if _, err := e.executor.Snapshot(context.Background()); err != nil {
							b.Fatal(err)
						}
					}
					e.CommitOperation("Unchanged gesture")
				}
				b.StopTimer()
				snapshot, err := e.SaveSnapshot(context.Background())
				if err != nil {
					b.Fatal(err)
				}
				got, err := snapshot.Hash()
				if err != nil || got != want || snapshot.Revision != 0 || e.app.CommandStorage().HasUndoV("test") {
					b.Fatal("unchanged workload changed authority or history")
				}
				b.Logf("fixture_cells=%d sha256=%s", cells, got)
			})
		}
	}
}

func TestUnchangedGestureDoesNotReadFullSnapshot(t *testing.T) {
	e := selectionEditor(t)
	counted := &countedSnapshotExecutor{Executor: e.executor}
	e.executor = counted
	coord := util.Point{X: 1, Y: 1, Z: 1}
	e.BeginTileChange(coord)
	instance := e.dmm.GetTile(coord).Instances()[2]
	before := instance.Prefab()
	instance.SetPrefab(dmmprefab.New(dmmprefab.IdNone, before.Path(), dmvars.Set(before.Vars(), "dir", "4")))
	instance.SetPrefab(before)
	e.CommitOperation("Cancelled edit")
	if counted.snapshots != 0 {
		t.Fatalf("unchanged gesture requested %d full snapshots", counted.snapshots)
	}
	if len(e.pendingChanges) != 0 || e.app.CommandStorage().HasUndoV("test") {
		t.Fatal("unchanged gesture retained pending work or history")
	}
}
