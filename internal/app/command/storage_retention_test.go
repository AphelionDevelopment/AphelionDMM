package command

import (
	"fmt"
	"runtime"
	"testing"
	"weak"
)

type historyPayload [8 << 20]byte

func pushHistoryPayload(storage *Storage, asynchronous bool) weak.Pointer[historyPayload] {
	payload := new(historyPayload)
	for index := 0; index < len(payload); index += 4096 {
		payload[index] = 1
	}
	use := func() { runtime.KeepAlive(payload) }
	if asynchronous {
		storage.Push(MakeAsync("payload", func(done func(error)) { use(); done(nil) }, func(done func(error)) { use(); done(nil) }))
	} else {
		storage.Push(Make("payload", use, use))
	}
	return weak.Make(payload)
}

func TestDiscardedHistoryReleasesPayloads(t *testing.T) {
	for _, asynchronous := range []bool{false, true} {
		t.Run(fmt.Sprintf("async=%v", asynchronous), func(t *testing.T) {
			storage := NewStorage()
			storage.SetStack("map")
			var payloads []weak.Pointer[historyPayload]
			for range 4 {
				payloads = append(payloads, pushHistoryPayload(storage, asynchronous))
			}
			for range 4 {
				storage.UndoV("map")
			}
			for range 4 {
				storage.RedoV("map")
			}
			for range 4 {
				storage.UndoV("map")
			}
			storage.Push(Make("new branch", func() {}, func() {}))
			runtime.GC()
			retained := 0
			for _, pointer := range payloads {
				if pointer.Value() != nil {
					retained++
				}
			}
			runtime.KeepAlive(storage)
			if retained != 0 {
				t.Fatalf("discarded history retains %d of four 8 MiB payloads (async=%v)", retained, asynchronous)
			}
			t.Log("all four discarded 8 MiB payloads were collected")
		})
	}
}

func TestLiveHistoryRetainsPayloadThroughBalance(t *testing.T) {
	storage := NewStorage()
	storage.SetStack("map")
	pointer := pushHistoryPayload(storage, false)
	storage.Balance("map")
	runtime.GC()
	if pointer.Value() == nil || !storage.HasRedoV("map") {
		t.Fatal("Balance released a live redo command")
	}
	storage.RedoV("map")
	storage.ForceBalance("map")
	storage.UndoV("map")
	storage.Balance("map")
	runtime.GC()
	if pointer.Value() == nil || !storage.HasUndoV("map") {
		t.Fatal("Balance released a live undo command")
	}
	storage.UndoV("map")
	storage.Push(Make("new branch", func() {}, func() {}))
	runtime.GC()
	if pointer.Value() != nil {
		t.Fatal("Balance retained a discarded command in unused slice capacity")
	}
	runtime.KeepAlive(storage)
}

func TestBalanceDoesNotCopyRedoClosuresIntoUnusedUndoSlots(t *testing.T) {
	storage := NewStorage()
	storage.SetStack("map")
	for range 4 {
		storage.Push(MakeAsync("edit", func(done func(error)) { done(nil) }, func(done func(error)) { done(nil) }))
	}
	storage.UndoV("map")
	storage.UndoV("map")
	stack := storage.commandStacks["map"]
	// Clear the old pop behavior's residue to isolate Balance's own copying.
	clear(stack.undo[len(stack.undo):cap(stack.undo)])
	storage.Balance("map")
	for _, command := range stack.undo[len(stack.undo):cap(stack.undo)] {
		if command.undoAsync != nil || command.redoAsync != nil {
			t.Fatal("Balance retained redo closures in unused undo capacity")
		}
	}
}
