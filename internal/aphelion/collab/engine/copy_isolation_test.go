package engine

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/model"
)

func TestDocumentCopyBranchesAreIndependent(t *testing.T) {
	parent, first, _ := prepareCopyWorkload(t, copyWorkload{100, 0, 1, 1})
	accepted, err := parent.Apply(first, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	before := parent.Snapshot()
	branches := []*Document{parent.Clone(), parent.Clone(), parent}
	operations := make([]model.Operation, len(branches))
	for index := range operations {
		operations[index] = copyOperation(t, before, index+2, 2)
		operations[index].Changes = operations[index].Changes[1:]
		operations[index].Changes[0].After.Prefabs[0].Vars["opaque"] = fmt.Sprint(index)
	}
	results := make(chan error, len(branches))
	for index, branch := range branches {
		go func() {
			_, err := branch.Apply(operations[index], time.Unix(2, 0))
			results <- err
		}()
	}
	for range branches {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	for index, branch := range branches {
		got := branch.Snapshot()
		if got.Tiles[1].State.Prefabs[0].Vars["opaque"] != fmt.Sprint(index) {
			t.Fatal("separate branches overwrote each other's tile state")
		}
		inverseID := model.OperationID(fmt.Sprintf("01890f3e-7b5c-7abc-8def-%012x", 20+index))
		inverse, err := branch.BuildInverse(testActorID, accepted.OperationID, inverseID)
		if err != nil {
			t.Fatal(err)
		}
		inverse.Changes[0].Before.Prefabs[0].Vars["dir"] = "caller mutation"
		duplicate, err := branch.Apply(first, time.Unix(3, 0))
		if err != nil || !reflect.DeepEqual(duplicate, accepted) {
			t.Fatal("caller mutation changed retained accepted history")
		}
		duplicate.Changes[0].After.Prefabs[0].Vars["dir"] = "returned result mutation"
		got.Tiles[0].State.Prefabs[0].Vars["dir"] = "snapshot mutation"
	}
	for index, branch := range branches {
		inverse, err := branch.BuildInverse(testActorID, accepted.OperationID, model.OperationID(fmt.Sprintf("01890f3e-7b5c-7abc-8def-%012x", 30+index)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := branch.Apply(inverse, time.Unix(4, 0)); err != nil {
			t.Fatal(err)
		}
		// Each branch must still be able to invert the common historical edit.
		if branch.Snapshot().Tiles[0].State.Prefabs[0].Vars["dir"] != "2" {
			t.Fatal("branch inverse lost original state")
		}
	}
}

func TestDocumentCopyFailureDoesNotMutateSharedState(t *testing.T) {
	for _, failure := range []string{"later precondition", "duplicate resulting ID"} {
		t.Run(failure, func(t *testing.T) {
			parent, operation, _ := prepareCopyWorkload(t, copyWorkload{100, 0, 1, 2})
			before := parent.Snapshot()
			branch := parent.Clone()
			if failure == "later precondition" {
				operation.Changes[1].Before.Prefabs[0].Vars["dir"] = "bad precondition"
			} else {
				operation.Changes[1].After.Prefabs[0].StableID = before.Tiles[2].State.Prefabs[0].StableID
			}
			if _, err := branch.Apply(operation, time.Unix(1, 0)); err == nil {
				t.Fatal("invalid batch was accepted")
			}
			if !reflect.DeepEqual(parent.Snapshot(), before) || !reflect.DeepEqual(branch.Snapshot(), before) {
				t.Fatal("failed batch mutated a document sharing the old state")
			}
		})
	}
}

func TestDocumentCopySparseAppendDoesNotOverwriteSibling(t *testing.T) {
	snapshot := initialSnapshot()
	snapshot.MaxX = 4
	parent, err := NewDocument(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	makeAppend := func(base model.Snapshot, x, serial int) model.Operation {
		after := model.CloneTileState(tileOneBefore())
		after.Prefabs[0].StableID = model.StableID(fmt.Sprintf("01890f3e-7b5c-7abc-8def-%012x", 100+serial))
		return operationFor(t, base, model.OperationID(fmt.Sprintf("01890f3e-7b5c-7abc-8def-%012x", serial)), []model.TileChange{{Coord: model.Coord{X: x, Y: 1, Z: 1}, After: after}})
	}
	if _, err := parent.Apply(makeAppend(snapshot, 3, 1), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	base := parent.Snapshot()
	left, right := parent.Clone(), parent.Clone()
	if _, err := left.Apply(makeAppend(base, 4, 2), time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}
	leftState := left.Snapshot()
	if _, err := right.Apply(makeAppend(base, 4, 3), time.Unix(3, 0)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left.Snapshot(), leftState) || !reflect.DeepEqual(parent.Snapshot(), base) {
		t.Fatal("append into spare capacity overwrote a sibling document")
	}
}
