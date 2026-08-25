package ui

import (
	"errors"
	"testing"

	"sdmm/internal/aphelion/collab/client"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

func TestBuildViewModelCapabilitiesOrderingAndAccessibleStatus(t *testing.T) {
	t.Parallel()

	view := BuildViewModel(SessionStatus{
		SessionID:      "session-1",
		Role:           "owner",
		State:          client.StateCaughtUp,
		Revision:       12,
		InviteReady:    true,
		ReconnectReady: true,
		Participants: []protocol.ParticipantPresence{
			{ActorID: model.ActorID("01890f3e-7b5c-7abc-8def-0123456789bb"), DisplayName: "Zed"},
			{ActorID: model.ActorID("01890f3e-7b5c-7abc-8def-0123456789bc"), DisplayName: "Alpha"},
			{ActorID: model.ActorID("01890f3e-7b5c-7abc-8def-0123456789bd")},
		},
	})
	if view.SyncLabel != "Caught up" || view.RevisionLabel != "Revision 12" {
		t.Fatalf("status labels = %q, %q", view.SyncLabel, view.RevisionLabel)
	}
	if !view.CanEdit || !view.CanAdminister || !view.CanCopyInvite || !view.CanLeave || view.CanReconnect {
		t.Fatalf("capabilities = %#v", view)
	}
	if len(view.Participants) != 3 || view.Participants[0].Label != "Alpha" || view.Participants[1].Label != "Zed" || view.Participants[2].Label == "" {
		t.Fatalf("participants = %#v", view.Participants)
	}
}

func TestBuildViewModelBoundsConflictsAndRedactsErrors(t *testing.T) {
	t.Parallel()

	conflicts := make([]client.Conflict, maxVisibleConflicts+2)
	for index := range conflicts {
		conflicts[index] = client.Conflict{OperationID: model.OperationID("01890f3e-7b5c-7abc-8def-0123456789bb"), Code: "precondition_failed", Message: "authoritative value changed"}
	}
	view := BuildViewModel(SessionStatus{State: client.StateConflict, Role: "viewer", Conflicts: conflicts, Err: errors.New("request used secret-token"), SensitiveValues: []string{"secret-token"}})
	if len(view.ConflictSummaries) != maxVisibleConflicts || view.HiddenConflictCount != 2 {
		t.Fatalf("conflict bounds = %d visible, %d hidden", len(view.ConflictSummaries), view.HiddenConflictCount)
	}
	if view.ErrorText != "request used [redacted]" || view.CanEdit || view.CanAdminister {
		t.Fatalf("error/capabilities = %q %#v", view.ErrorText, view)
	}
}
