package engine

import (
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/model"
)

func TestDocumentConformance(t *testing.T) {
	t.Parallel()

	t.Run("authentic stale operation merges on untouched tile", func(t *testing.T) {
		t.Parallel()
		initial := initialSnapshot()
		document, err := NewDocument(initial)
		if err != nil {
			t.Fatalf("NewDocument() error = %v", err)
		}

		first := operationFor(t, initial, "01890f3e-7b5c-7abc-8def-0123456789bb", []model.TileChange{{
			Coord: model.Coord{X: 1, Y: 1, Z: 1}, Before: tileOneBefore(), After: model.TileState{},
		}})
		if _, err := document.Apply(first, time.Unix(1, 0)); err != nil {
			t.Fatalf("first Apply() error = %v", err)
		}

		stale := operationFor(t, initial, "01890f3e-7b5c-7abc-8def-0123456789bc", []model.TileChange{{
			Coord: model.Coord{X: 2, Y: 1, Z: 1}, Before: model.TileState{}, After: model.TileState{Prefabs: []model.PrefabState{{
				StableID: "01890f3e-7b5c-7abc-8def-0123456789ae",
				Path:     "/obj/foo3",
			}}},
		}})
		accepted, err := document.Apply(stale, time.Unix(2, 0))
		if err != nil {
			t.Fatalf("stale Apply() error = %v", err)
		}
		if accepted.Revision != initial.Revision+2 {
			t.Fatalf("stale accepted revision = %d, want %d", accepted.Revision, initial.Revision+2)
		}
	})

	t.Run("accepted changes use coordinate order", func(t *testing.T) {
		t.Parallel()
		initial := initialSnapshot()
		document, err := NewDocument(initial)
		if err != nil {
			t.Fatalf("NewDocument() error = %v", err)
		}
		operation := operationFor(t, initial, "01890f3e-7b5c-7abc-8def-0123456789bb", []model.TileChange{
			{Coord: model.Coord{X: 2, Y: 1, Z: 1}, Before: model.TileState{}, After: model.TileState{}},
			{Coord: model.Coord{X: 1, Y: 1, Z: 1}, Before: tileOneBefore(), After: tileOneBefore()},
		})
		accepted, err := document.Apply(operation, time.Now())
		if err != nil {
			t.Fatalf("Apply() error = %v", err)
		}
		if accepted.Changes[0].Coord.X != 1 || accepted.Changes[1].Coord.X != 2 {
			t.Fatalf("accepted coordinate order = %#v", accepted.Changes)
		}
	})
}
