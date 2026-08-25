package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/model"
)

func TestHubCreateJoinLeaveAndAuthorization(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 2)
	ownerLoop, err := StartDocument(context.Background(), snapshot, NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ownerLoop.Close(context.Background()) })

	owner := testPrincipal(t, "Owner", RoleOwner)
	editor := testPrincipal(t, "Editor", RoleEditor)
	viewer := testPrincipal(t, "Viewer", RoleViewer)
	hub := NewHub(time.Minute)
	if err := hub.Create("session-1", ownerLoop, owner); err != nil {
		t.Fatal(err)
	}
	if err := hub.Join("session-1", editor); err != nil {
		t.Fatal(err)
	}
	if err := hub.Join("session-1", viewer); err != nil {
		t.Fatal(err)
	}

	operation := testOperation(t, snapshot, 1)
	operation.ActorID = viewer.ActorID()
	if _, err := hub.Submit(context.Background(), "session-1", viewer, operation); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("viewer Submit() error = %v, want %v", err, ErrUnauthorized)
	}
	operation.ActorID = viewer.ActorID()
	accepted, err := hub.Submit(context.Background(), "session-1", editor, operation)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.ActorID != editor.ActorID() {
		t.Fatalf("accepted actor = %q, want derived actor %q", accepted.ActorID, editor.ActorID())
	}

	if err := hub.Leave("session-1", editor.ActorID()); err != nil {
		t.Fatal(err)
	}
	operation = testOperation(t, snapshot, 2)
	if _, err := hub.Submit(context.Background(), "session-1", editor, operation); !errors.Is(err, ErrNotJoined) {
		t.Fatalf("departed editor Submit() error = %v, want %v", err, ErrNotJoined)
	}
}

func TestPresencePressureDoesNotBlockDurableDelivery(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	ownerLoop, err := StartDocument(context.Background(), snapshot, NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ownerLoop.Close(context.Background()) })
	principal := testPrincipal(t, "Owner", RoleOwner)
	hub := NewHub(time.Minute)
	if err := hub.Create("session-1", ownerLoop, principal); err != nil {
		t.Fatal(err)
	}
	durable, cancelDurable, err := hub.SubscribeDurable("session-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelDurable()
	_, presence, cancelPresence, err := hub.SubscribePresence("session-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelPresence()

	for sequence := uint64(1); sequence <= 1000; sequence++ {
		if err := hub.UpdatePresence("session-1", principal, PresenceUpdate{Sequence: sequence, Status: "active"}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case latest := <-presence:
		if latest.Sequence != 1000 {
			t.Fatalf("coalesced sequence = %d, want 1000", latest.Sequence)
		}
	default:
		t.Fatal("presence subscriber received no coalesced update")
	}

	accepted, err := hub.Submit(context.Background(), "session-1", principal, testOperation(t, snapshot, 1))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case delivered := <-durable:
		if delivered.OperationID != accepted.OperationID {
			t.Fatalf("durable operation = %q, want %q", delivered.OperationID, accepted.OperationID)
		}
	case <-time.After(time.Second):
		t.Fatal("durable delivery blocked by presence pressure")
	}
}

func testPrincipal(t *testing.T, displayName string, role Role) Principal {
	t.Helper()
	actorID, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	principal, err := NewPrincipal("principal-"+displayName, actorID, displayName, role)
	if err != nil {
		t.Fatal(err)
	}
	return principal
}
