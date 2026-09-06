package client

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"sdmm/internal/aphelion/collab/model"
)

func indexedFixture(cells int) model.Snapshot {
	s := model.Snapshot{ProtocolVersion: model.ProtocolVersion, SchemaVersion: model.SchemaVersion, DocumentID: "01890f3e-7b5c-7abc-8def-0123456789ab", EnvironmentHash: strings.Repeat("a", 64), MaxX: cells + 1, MaxY: 1, MaxZ: 1}
	for i := 1; i <= cells; i++ {
		s.Tiles = append(s.Tiles, model.Tile{Coord: model.Coord{X: i, Y: 1, Z: 1}, State: model.TileState{Prefabs: []model.PrefabState{{StableID: model.StableID(fmt.Sprintf("01890f3e-7b5c-7abc-8def-%012x", i)), Path: "/obj/unknown", Vars: map[string]string{"dir": "2", "opaque": "list(1, /missing/type)"}}}}})
	}
	return s
}

func referenceVisible(p Projection) model.Snapshot {
	s := model.CloneSnapshot(p.Acknowledged)
	for _, op := range p.Pending {
		if next, err := applyOperation(s, op); err == nil {
			s = next
		}
	}
	s.Revision = p.Acknowledged.Revision
	return s
}

func TestIndexedProjectionMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(260906))
	for sequence := 0; sequence < 40; sequence++ {
		p := Projection{Acknowledged: indexedFixture(20)}
		for step := 0; step < 16; step++ {
			visible := referenceVisible(p)
			index := r.Intn(len(visible.Tiles))
			tile := visible.Tiles[index]
			after := model.CloneTileState(tile.State)
			if len(after.Prefabs) == 0 {
				after = indexedFixture(1).Tiles[0].State
			} else {
				after.Prefabs[0].Vars["dir"] = fmt.Sprint(step)
			}
			change := model.TileChange{Coord: tile.Coord, Before: model.CloneTileState(tile.State), After: after}
			op := model.Operation{Changes: []model.TileChange{change}}
			switch r.Intn(7) {
			case 0:
				op.Changes = append(op.Changes, change) // Duplicate coordinate.
			case 1:
				op.Changes[0].After.Prefabs[0].StableID = "invalid"
			case 2:
				op.Changes[0].Before = model.TileState{Prefabs: []model.PrefabState{{Path: "/wrong"}}}
			case 3: // Valid ID relocation, evaluated as one batch, in either order.
				other := visible.Tiles[(index+1)%len(visible.Tiles)]
				op.Changes[0].After = model.CloneTileState(other.State)
				op.Changes = append(op.Changes, model.TileChange{Coord: other.Coord, Before: model.CloneTileState(other.State), After: model.CloneTileState(tile.State)})
			case 4:
				op.Changes[0].After = model.TileState{}
			case 5:
				op.Changes = append(op.Changes, model.TileChange{Coord: model.Coord{X: 999, Y: 1, Z: 1}})
			}
			p.Pending = append(p.Pending, op)
			before := cloneProjection(p)
			want := referenceVisible(p)
			got, err := p.Visible()
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("sequence=%d step=%d differs: err=%v\ngot=%#v\nwant=%#v", sequence, step, err, got, want)
			}
			if !reflect.DeepEqual(p, before) {
				t.Fatal("projection mutated its source or pending requests")
			}
		}
	}
}

func TestIndexedProjectionAppendCollisionAndIsolation(t *testing.T) {
	p := Projection{Acknowledged: indexedFixture(2)}
	first := p.Acknowledged.Tiles[0]
	destination := model.Coord{X: 3, Y: 1, Z: 1}
	p.Pending = []model.Operation{{Changes: []model.TileChange{
		{Coord: destination, After: model.CloneTileState(first.State)},
		{Coord: first.Coord, Before: model.CloneTileState(first.State)},
	}}}
	// This would partially overwrite the other tile and duplicate the moved ID.
	p.Pending = append(p.Pending, model.Operation{Changes: []model.TileChange{{Coord: p.Acknowledged.Tiles[1].Coord, Before: model.CloneTileState(p.Acknowledged.Tiles[1].State), After: model.CloneTileState(first.State)}}})
	before := cloneProjection(p)
	got, err := p.Visible()
	if err != nil || !reflect.DeepEqual(got, referenceVisible(p)) {
		t.Fatalf("append/collision: %v", err)
	}
	got.Tiles[1].State.Prefabs[0].Vars["opaque"] = "mutated result"
	got.Tiles[2].State.Prefabs[0].Vars["opaque"] = "mutated pending result"
	if !reflect.DeepEqual(p, before) {
		t.Fatal("visible state aliases acknowledged or submitted data")
	}
	// Public Projection values can be malformed: preserve existing fallback behavior.
	p.Acknowledged.Tiles[0].State.Prefabs[0].StableID = "bad"
	got, err = p.Visible()
	if err != nil || !reflect.DeepEqual(got, referenceVisible(p)) {
		t.Fatal("invalid baseline changed behavior")
	}
}

func TestProjectionAllocationDoesNotMultiplyWithPendingMapCopies(t *testing.T) {
	p := Projection{Acknowledged: indexedFixture(1000)}
	base := testing.AllocsPerRun(2, func() {
		if _, err := p.Visible(); err != nil {
			t.Fatal(err)
		}
	})
	for i := 0; i < 8; i++ {
		tile := p.Acknowledged.Tiles[i]
		after := model.CloneTileState(tile.State)
		after.Prefabs[0].Vars["dir"] = "4"
		p.Pending = append(p.Pending, model.Operation{Changes: []model.TileChange{{Coord: tile.Coord, Before: tile.State, After: after}}})
	}
	many := testing.AllocsPerRun(2, func() {
		if _, err := p.Visible(); err != nil {
			t.Fatal(err)
		}
	})
	if many-base > 300 {
		t.Fatalf("eight one-tile edits added %.0f allocations to a %.0f-allocation projection", many-base, base)
	}
}
