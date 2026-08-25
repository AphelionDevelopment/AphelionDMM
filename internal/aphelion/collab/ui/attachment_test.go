package ui

import (
	"context"
	"testing"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/executor"
	"sdmm/internal/aphelion/collab/model"
)

func TestPrepareLocalSessionReturnsCreatedExecutor(t *testing.T) {
	t.Parallel()

	snapshot := controllerSnapshot(t)
	execution := attachmentExecutor(t, snapshot)
	session := &fakeLocalSession{}
	provider := fakeExecutorProvider{execution: execution}

	prepared, err := PrepareLocalSession(context.Background(), session, provider, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if prepared != execution {
		t.Fatal("prepared session returned a different executor")
	}
	if !session.created || session.snapshot.DocumentID != snapshot.DocumentID || session.leaveCalls != 0 {
		t.Fatalf("session state = created %t document %q leaves %d", session.created, session.snapshot.DocumentID, session.leaveCalls)
	}
}

func TestPrepareLocalSessionLeavesIncompleteSession(t *testing.T) {
	t.Parallel()

	session := &fakeLocalSession{}
	if _, err := PrepareLocalSession(context.Background(), session, fakeExecutorProvider{}, controllerSnapshot(t)); err == nil {
		t.Fatal("prepare local session accepted a missing executor")
	}
	if session.leaveCalls != 1 {
		t.Fatalf("incomplete session leave calls = %d, want 1", session.leaveCalls)
	}
}

func TestAttachPreparedSessionRefusesChangedTarget(t *testing.T) {
	t.Parallel()

	execution := attachmentExecutor(t, controllerSnapshot(t))
	target := &fakeAttachmentTarget{}
	if err := AttachPreparedSession(execution, target, false); err == nil {
		t.Fatal("attachment accepted a changed editor target")
	}
	if target.execution != nil {
		t.Fatal("changed editor target received the collaboration executor")
	}
	if err := AttachPreparedSession(execution, target, true); err != nil {
		t.Fatal(err)
	}
	if target.execution != execution {
		t.Fatal("current editor target did not receive the collaboration executor")
	}
}

func attachmentExecutor(t *testing.T, snapshot model.Snapshot) executor.Executor {
	t.Helper()
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	actorID, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	local, err := executor.NewLocal(document, actorID)
	if err != nil {
		t.Fatal(err)
	}
	return local
}

type fakeLocalSession struct {
	created    bool
	snapshot   model.Snapshot
	leaveCalls int
}

func (session *fakeLocalSession) CreateLocal(_ context.Context, snapshot model.Snapshot) error {
	session.created = true
	session.snapshot = model.CloneSnapshot(snapshot)
	return nil
}

func (session *fakeLocalSession) Leave(context.Context) error {
	session.leaveCalls++
	return nil
}

type fakeExecutorProvider struct {
	execution executor.Executor
}

func (provider fakeExecutorProvider) CollaborationExecutor() executor.Executor {
	return provider.execution
}

type fakeAttachmentTarget struct {
	execution executor.Executor
}

func (target *fakeAttachmentTarget) AttachCollaborationExecutor(execution executor.Executor) error {
	target.execution = execution
	return nil
}
