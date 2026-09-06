package command

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"testing"
)

func TestAcceptedEditDuringHistoryAcknowledgement(t *testing.T) {
	for _, redo := range []bool{false, true} {
		for _, reject := range []bool{false, true} {
			t.Run(fmt.Sprintf("redo=%v/reject=%v", redo, reject), func(t *testing.T) {
				storage := NewStorage()
				storage.SetStack("map")
				a, b := true, false
				var acknowledge func(error)
				action := func(applied bool) func(func(error)) {
					return func(done func(error)) {
						acknowledge = func(err error) {
							if err == nil {
								a = applied
							}
							done(err)
						}
					}
				}
				storage.Push(MakeAsync("A", action(false), action(true)))
				storage.UndoV("map")
				if redo {
					acknowledge(nil)
					storage.RedoV("map")
				}
				b = true // An independently accepted edit arrives before history completion.
				storage.Push(Make("B", func() { b = false }, func() { b = true }))
				if storage.HasUndoV("map") || storage.HasRedoV("map") {
					t.Fatal("pending history became actionable")
				}
				var result error
				if reject {
					result = errors.New("controlled history rejection")
				}
				acknowledge(result)
				storage.UndoV("map")
				if b {
					t.Fatal("latest accepted edit was lost from undo history")
				}
				if storage.HasUndoV("map") != a {
					t.Fatalf("history does not match durable A after undoing B: undo=%v A=%v", storage.HasUndoV("map"), a)
				}
				if a {
					storage.UndoV("map")
					acknowledge(nil)
				}
				if a || b || storage.HasUndoV("map") {
					t.Fatal("accepted history cannot return to the initial state")
				}
			})
		}
	}
}

func TestSavedHistoryDetectsReplacementAtSameDepth(t *testing.T) {
	storage := NewStorage()
	storage.SetStack("map")
	for _, label := range []string{"A", "B"} {
		storage.Push(Make(label, func() {}, func() {}))
	}
	storage.ForceBalance("map")
	storage.UndoV("map")
	storage.Push(Make("C", func() {}, func() {}))
	if !storage.IsModified("map") {
		t.Fatal("a different edit at the saved history depth is marked clean")
	}
	storage.ForceBalance("map")
	storage.UndoV("map")
	if !storage.IsModified("map") {
		t.Fatal("undo away from saved history is marked clean")
	}
	storage.RedoV("map")
	if storage.IsModified("map") {
		t.Fatal("redo to the exact saved command is marked dirty")
	}
}

func TestPendingHistoryIsModifiedUntilRejected(t *testing.T) {
	storage := NewStorage()
	storage.SetStack("map")
	var acknowledge func(error)
	storage.Push(MakeAsync("saved edit", func(done func(error)) { acknowledge = done }, func(done func(error)) { done(nil) }))
	storage.ForceBalance("map")
	storage.UndoV("map")
	if !storage.IsModified("map") {
		t.Error("pending inverse is marked clean")
	}
	acknowledge(errors.New("controlled rejection"))
	if storage.IsModified("map") {
		t.Fatal("rejected inverse dirtied unchanged saved history")
	}
}

func TestHistoryQueuedEditsPreserveOrder(t *testing.T) {
	storage := NewStorage()
	storage.SetStack("map")
	var acknowledge func(error)
	storage.Push(MakeAsync("A", func(done func(error)) { acknowledge = done }, func(done func(error)) { done(nil) }))
	completed := 0
	storage.UndoAsyncV("map", func(error) { completed++ })
	var undone []string
	for _, label := range []string{"B", "C", "D"} {
		storage.Push(Make(label, func() { undone = append(undone, label) }, func() {}))
	}
	acknowledge(nil)
	acknowledge(nil) // A duplicate provider completion must not flush or finish twice.
	for range 3 {
		storage.UndoV("map")
	}
	if completed != 1 || !reflect.DeepEqual(undone, []string{"D", "C", "B"}) || storage.HasUndoV("map") {
		t.Fatalf("history order/completion changed: undo=%v callbacks=%d", undone, completed)
	}
}

func TestDisposedHistoryReleasesQueuedPayloads(t *testing.T) {
	storage := NewStorage()
	storage.SetStack("map")
	target := storage.Bind("map")
	var acknowledge func(error)
	storage.Push(MakeAsync("pending undo", func(done func(error)) { acknowledge = done }, func(done func(error)) { done(nil) }))
	storage.UndoV("map")
	payload := pushHistoryPayload(storage, true)
	storage.DisposeStack("map")
	runtime.GC()
	if payload.Value() != nil {
		t.Fatal("disposed stack retained an edit queued during undo")
	}
	acknowledge(nil)
	runtime.KeepAlive(target)
}
