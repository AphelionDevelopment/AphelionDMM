package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	collabclient "sdmm/internal/aphelion/collab/client"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
	"sdmm/internal/aphelion/collab/server"
)

func TestSessionClientDropsPresenceOnConnectionLoss(t *testing.T) {
	t.Parallel()

	client := NewSessionClient(SessionClientConfig{})
	machine := client.machine
	if err := machine.Apply(collabclient.EventConnect); err != nil {
		t.Fatal(err)
	}
	client.participants[model.ActorID("01890f3e-7b5c-7abc-8def-0123456789ac")] = ObservedPresence{
		Presence:   protocol.ParticipantPresence{ActorID: model.ActorID("01890f3e-7b5c-7abc-8def-0123456789ac")},
		ObservedAt: time.Now(),
	}

	client.recordConnectionFailure(machine, errors.New("connection lost"))

	if presence := client.ObservedPresence(); len(presence) != 0 {
		t.Fatalf("presence after connection loss = %d entries, want 0", len(presence))
	}
}

func TestSessionClientReportsConnectionLoss(t *testing.T) {
	t.Parallel()

	snapshot := controllerSnapshot(t)
	embedded, err := server.StartEmbedded(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	client := NewSessionClient(SessionClientConfig{HTTPTimeout: time.Second})
	invitation, err := client.Create(context.Background(), embedded.Endpoint(), embedded.TakeLaunchToken(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Join(context.Background(), invitation); err != nil {
		t.Fatal(err)
	}
	if err := embedded.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if client.Status().State == collabclient.StateReconnecting {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state after connection loss = %q, want reconnecting", client.Status().State)
}
