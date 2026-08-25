package command

import (
	"errors"
	"testing"
)

func TestUndoAsyncMovesHistoryOnlyAfterAcknowledgement(t *testing.T) {
	storage := NewStorage()
	storage.SetStack("map")
	var acknowledge func(error)
	storage.Push(MakeAsync("network edit", func(complete func(error)) {
		acknowledge = complete
	}, func(complete func(error)) {
		complete(nil)
	}))

	var completed error
	if !storage.UndoAsyncV("map", func(err error) { completed = err }) {
		t.Fatal("UndoAsyncV did not start")
	}
	if acknowledge == nil {
		t.Fatal("async undo was not invoked")
	}
	if storage.HasUndoV("map") || storage.HasRedoV("map") {
		t.Fatal("pending undo remained actionable")
	}

	acknowledge(nil)
	if completed != nil {
		t.Fatalf("async undo completion error = %v", completed)
	}
	if storage.HasUndoV("map") || !storage.HasRedoV("map") {
		t.Fatal("acknowledged undo did not move command to redo history")
	}
}

func TestUndoAsyncRejectionPreservesUndoHistory(t *testing.T) {
	storage := NewStorage()
	storage.SetStack("map")
	var acknowledge func(error)
	storage.Push(MakeAsync("network edit", func(complete func(error)) {
		acknowledge = complete
	}, func(complete func(error)) {
		complete(nil)
	}))

	rejected := errors.New("server rejected inverse")
	var completed error
	if !storage.UndoAsyncV("map", func(err error) { completed = err }) {
		t.Fatal("UndoAsyncV did not start")
	}
	if storage.UndoAsyncV("map", nil) {
		t.Fatal("second undo started while acknowledgement was pending")
	}
	acknowledge(rejected)
	if !errors.Is(completed, rejected) {
		t.Fatalf("completion error = %v, want %v", completed, rejected)
	}
	if !storage.HasUndoV("map") || storage.HasRedoV("map") {
		t.Fatal("rejected undo changed command history")
	}
}

func TestRedoAsyncMovesHistoryOnlyAfterAcknowledgement(t *testing.T) {
	storage := NewStorage()
	storage.SetStack("map")
	var acknowledgeRedo func(error)
	storage.Push(MakeAsync("network edit", func(complete func(error)) {
		complete(nil)
	}, func(complete func(error)) {
		acknowledgeRedo = complete
	}))
	if !storage.UndoAsyncV("map", nil) {
		t.Fatal("UndoAsyncV did not start")
	}

	if !storage.RedoAsyncV("map", nil) {
		t.Fatal("RedoAsyncV did not start")
	}
	if acknowledgeRedo == nil {
		t.Fatal("async redo was not invoked")
	}
	if storage.HasUndoV("map") || storage.HasRedoV("map") {
		t.Fatal("pending redo remained actionable")
	}
	acknowledgeRedo(nil)
	if !storage.HasUndoV("map") || storage.HasRedoV("map") {
		t.Fatal("acknowledged redo did not move command to undo history")
	}
}

func TestBalanceDoesNotRewindAsynchronousHistory(t *testing.T) {
	storage := NewStorage()
	storage.SetStack("map")
	runs := 0
	storage.Push(MakeAsync("shared edit", func(func(error)) {
		runs++
	}, func(func(error)) {
		runs++
	}))

	storage.Balance("map")
	if runs != 0 {
		t.Fatalf("Balance executed %d shared history operations", runs)
	}
	if !storage.HasUndoV("map") {
		t.Fatal("Balance removed shared history")
	}
}
