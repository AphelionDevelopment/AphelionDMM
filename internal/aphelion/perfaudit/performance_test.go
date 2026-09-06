// Package perfaudit contains reproducible component workloads, not desktop or
// production capacity gates. No benchmark disables validation or durability.
package perfaudit

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/client"
	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/mapadapter"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/store/sqlite"
	"sdmm/internal/dmapi/dmmap/dmmdata"
)

func fixture(cells int) model.Snapshot {
	snapshot := model.Snapshot{ProtocolVersion: model.ProtocolVersion, SchemaVersion: model.SchemaVersion,
		DocumentID: "01890f3e-7b5c-7abc-8def-0123456789ab", EnvironmentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		MaxX: 10, MaxY: cells / 10, MaxZ: 1}
	for index := 0; index < cells; index++ {
		snapshot.Tiles = append(snapshot.Tiles, model.Tile{Coord: model.Coord{X: index%10 + 1, Y: index/10 + 1, Z: 1},
			State: model.TileState{Prefabs: []model.PrefabState{{StableID: model.StableID(fmt.Sprintf("01890f3e-7b5c-7abc-8def-%012x", index+1)), Path: "/obj/foo", Vars: map[string]string{"dir": "2"}}}}})
	}
	return snapshot
}

func operation(tb testing.TB, snapshot model.Snapshot, index int, serial int) model.Operation {
	tb.Helper()
	hash, err := snapshot.Hash()
	if err != nil {
		tb.Fatal(err)
	}
	tile := snapshot.Tiles[index]
	after := model.CloneTileState(tile.State)
	if after.Prefabs[0].Vars["dir"] == "2" {
		after.Prefabs[0].Vars["dir"] = "4"
	} else {
		after.Prefabs[0].Vars["dir"] = "2"
	}
	return model.Operation{ProtocolVersion: model.ProtocolVersion, DocumentID: snapshot.DocumentID,
		ActorID: "01890f3e-7b5c-7abc-8def-0123456789ba", OperationID: model.OperationID(fmt.Sprintf("01890f3e-7b5c-7abc-8def-%012x", serial)),
		BaseRevision: snapshot.Revision, BaseMapHash: hash, EnvironmentHash: snapshot.EnvironmentHash, Kind: model.OperationKindTileChange,
		Changes: []model.TileChange{{Coord: tile.Coord, Before: tile.State, After: after}}}
}

func document(tb testing.TB, cells, history int) *engine.Document {
	tb.Helper()
	doc, err := engine.NewDocument(fixture(cells))
	if err != nil {
		tb.Fatal(err)
	}
	for index := 0; index < history; index++ {
		if _, err := doc.Apply(operation(tb, doc.Snapshot(), 0, index+1), time.Unix(0, int64(index))); err != nil {
			tb.Fatal(err)
		}
	}
	return doc
}

func TestPerformanceWorkloads(t *testing.T) {
	for _, cells := range []int{100, 1000, 10000} {
		snapshot := fixture(cells)
		hash, err := snapshot.Hash()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("fixture cells=%d prefabs=%d hash=%s", cells, cells, hash)
		doc := document(t, cells, 2)
		before := doc.Snapshot()
		op := operation(t, before, 0, 3)
		clone := doc.Clone()
		accepted, err := clone.Apply(op, time.Unix(0, 3))
		if err != nil || accepted.Revision != 3 || !clone.Snapshot().Tiles[0].State.Equal(op.Changes[0].After) || !doc.Snapshot().Tiles[0].State.Equal(before.Tiles[0].State) {
			t.Fatalf("invalid mutation/reset workload: %v", err)
		}
		projection := client.NewProjection(snapshot)
		for index := 0; index < 8; index++ {
			projection, err = projection.Submit(operation(t, snapshot, index, index+1))
			if err != nil {
				t.Fatal(err)
			}
		}
		visible, err := projection.Visible()
		if err != nil {
			t.Fatal(err)
		}
		for index := 0; index < 8; index++ {
			if visible.Tiles[index].State.Prefabs[0].Vars["dir"] != "4" {
				t.Fatal("projection dropped work")
			}
		}
		path := filepath.Join(t.TempDir(), "roundtrip.dmm")
		data, err := mapadapter.Export(snapshot, path, false, "\n")
		if err != nil {
			t.Fatal(err)
		}
		if err := data.Save(); err != nil {
			t.Fatal(err)
		}
		parsed, err := dmmdata.New(path)
		if err != nil || len(parsed.Grid) != cells {
			t.Fatalf("invalid parser fixture: %v", err)
		}
	}
}

func BenchmarkAuditSnapshotHash(b *testing.B) {
	for _, cells := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(cells), func(b *testing.B) {
			snapshot := fixture(cells)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := snapshot.Hash(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAuditDocumentCloneApply(b *testing.B) {
	for _, pair := range [][2]int{{100, 0}, {1000, 0}, {10000, 0}, {100, 100}, {100, 1000}} {
		b.Run(fmt.Sprintf("cells=%d/history=%d", pair[0], pair[1]), func(b *testing.B) {
			doc := document(b, pair[0], pair[1])
			op := operation(b, doc.Snapshot(), 0, pair[1]+1)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				candidate := doc.Clone()
				accepted, err := candidate.Apply(op, time.Unix(0, 1))
				if err != nil || accepted.Revision != model.Revision(pair[1]+1) {
					b.Fatalf("apply: %v", err)
				}
			}
		})
	}
}

func BenchmarkAuditProjectionVisible(b *testing.B) {
	for _, count := range []int{0, 1, 8} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			snapshot := fixture(1000)
			projection := client.NewProjection(snapshot)
			for i := 0; i < count; i++ {
				var err error
				projection, err = projection.Submit(operation(b, snapshot, i, i+1))
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := projection.Visible(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAuditMapExport(b *testing.B) {
	for _, cells := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(cells), func(b *testing.B) {
			snapshot := fixture(cells)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := mapadapter.Export(snapshot, "map.dmm", false, "\n"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAuditMapParse(b *testing.B) {
	for _, cells := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(cells), func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "map.dmm")
			data, err := mapadapter.Export(fixture(cells), path, false, "\n")
			if err != nil {
				b.Fatal(err)
			}
			if err := data.Save(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				parsed, err := dmmdata.New(path)
				if err != nil || len(parsed.Grid) != cells {
					b.Fatalf("parse: %v", err)
				}
			}
		})
	}
}

// A fresh database with the same retained history precedes every timed append.
// Setup and close are excluded; use a fixed iteration count for this costly fixture.
func BenchmarkAuditSQLiteAppend(b *testing.B) {
	for _, history := range []int{0, 100} {
		b.Run(fmt.Sprint(history), func(b *testing.B) {
			ctx := context.Background()
			directory := b.TempDir()
			b.ReportAllocs()
			b.ResetTimer()
			b.StopTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				storage, err := sqlite.Open(filepath.Join(directory, fmt.Sprintf("%d.sqlite", iteration)))
				if err != nil {
					b.Fatal(err)
				}
				doc := document(b, 100, 0)
				if err := storage.Create(ctx, doc.Snapshot()); err != nil {
					b.Fatal(err)
				}
				for i := 0; i < history; i++ {
					accepted, err := doc.Apply(operation(b, doc.Snapshot(), 0, i+1), time.Unix(0, int64(i)))
					if err != nil {
						b.Fatal(err)
					}
					if err := storage.Append(ctx, accepted); err != nil {
						b.Fatal(err)
					}
				}
				if err := storage.SaveSnapshot(ctx, doc.Snapshot()); err != nil {
					b.Fatal(err)
				}
				accepted, err := doc.Apply(operation(b, doc.Snapshot(), 0, history+1), time.Unix(0, int64(history)))
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				err = storage.Append(ctx, accepted)
				b.StopTimer()
				if err != nil {
					b.Fatal(err)
				}
				recovery, err := storage.LoadRecovery(ctx, accepted.DocumentID)
				if err != nil {
					b.Fatal(err)
				}
				recovered, err := recovery.Restore()
				if err != nil {
					b.Fatal(err)
				}
				want, _ := doc.Snapshot().Hash()
				got, _ := recovered.Snapshot().Hash()
				if got != want || recovered.Snapshot().Revision != accepted.Revision {
					b.Fatal("append/recovery diverged")
				}
				if err := storage.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
