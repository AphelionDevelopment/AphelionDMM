package server

import (
	"testing"
	"time"
)

func TestPresenceCoalescesAndExpires(t *testing.T) {
	t.Parallel()

	manager := NewPresenceManager(30 * time.Second)
	principal := testPrincipal(t, "Editor", RoleEditor)
	base := time.Unix(100, 0)
	snapshot, updates, cancel := manager.Subscribe(1)
	defer cancel()
	if len(snapshot) != 0 {
		t.Fatalf("initial snapshot length = %d, want 0", len(snapshot))
	}

	if err := manager.updateAt(principal, PresenceUpdate{Sequence: 1, Status: "active"}, base); err != nil {
		t.Fatal(err)
	}
	if err := manager.updateAt(principal, PresenceUpdate{Sequence: 2, Status: "active"}, base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	latest := <-updates
	if latest.Sequence != 2 {
		t.Fatalf("coalesced sequence = %d, want 2", latest.Sequence)
	}
	if removed := manager.Expire(base.Add(31 * time.Second)); removed != 1 {
		t.Fatalf("Expire() removed %d presences, want 1", removed)
	}
	snapshot, _, cancelSnapshot := manager.Subscribe(1)
	defer cancelSnapshot()
	if len(snapshot) != 0 {
		t.Fatalf("snapshot after expiry length = %d, want 0", len(snapshot))
	}
}

func TestPresenceRejectsOldSequenceAndRemovesOnDisconnect(t *testing.T) {
	t.Parallel()

	manager := NewPresenceManager(time.Minute)
	principal := testPrincipal(t, "Editor", RoleEditor)
	if err := manager.updateAt(principal, PresenceUpdate{Sequence: 2, Status: "active"}, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if err := manager.Update(principal, PresenceUpdate{Sequence: 1, Status: "active"}); err == nil {
		t.Fatal("old presence sequence accepted")
	}
	manager.Remove(principal.ActorID())
	snapshot, _, cancel := manager.Subscribe(1)
	defer cancel()
	if len(snapshot) != 0 {
		t.Fatalf("snapshot after disconnect length = %d, want 0", len(snapshot))
	}
}
