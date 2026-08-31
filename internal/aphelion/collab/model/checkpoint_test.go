package model

import (
	"strings"
	"testing"
	"time"
)

func TestExportCheckpointValidatesPendingAndTerminalStates(t *testing.T) {
	t.Parallel()

	pending := exportCheckpointFixture()
	if err := pending.Validate(); err != nil {
		t.Fatalf("pending Validate() error = %v", err)
	}

	accepted, err := pending.Complete(ExportCheckpointCompletion{
		Status:          ExportCheckpointAccepted,
		ArtifactHash:    strings.Repeat("b", 64),
		Verifier:        "meridian-mcp",
		VerifierVersion: "0.2.0",
		CompletedAt:     time.Unix(20, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Complete(accepted) error = %v", err)
	}
	if accepted.Status != ExportCheckpointAccepted || accepted.CompletedAt == nil {
		t.Fatalf("accepted checkpoint = %#v", accepted)
	}
	if _, err := accepted.Complete(ExportCheckpointCompletion{Status: ExportCheckpointRejected}); err == nil {
		t.Fatal("Complete(terminal checkpoint) error = nil")
	}

	rejected, err := pending.Complete(ExportCheckpointCompletion{
		Status:          ExportCheckpointRejected,
		Verifier:        "meridian-mcp",
		VerifierVersion: "0.2.0",
		DiagnosticCode:  "dreammaker_failed",
		CompletedAt:     time.Unix(20, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Complete(rejected) error = %v", err)
	}
	if rejected.Status != ExportCheckpointRejected || rejected.DiagnosticCode != "dreammaker_failed" {
		t.Fatalf("rejected checkpoint = %#v", rejected)
	}
}

func TestExportCheckpointRejectsInvalidState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*ExportCheckpoint)
		wantErr string
	}{
		{name: "empty checkpoint id", mutate: func(value *ExportCheckpoint) { value.CheckpointID = "" }, wantErr: "checkpoint id"},
		{name: "empty idempotency key", mutate: func(value *ExportCheckpoint) { value.IdempotencyKey = "" }, wantErr: "idempotency key"},
		{name: "oversized idempotency key", mutate: func(value *ExportCheckpoint) { value.IdempotencyKey = strings.Repeat("x", MaxCheckpointTextBytes+1) }, wantErr: "idempotency key"},
		{name: "empty session", mutate: func(value *ExportCheckpoint) { value.SessionID = "" }, wantErr: "session id"},
		{name: "invalid map hash", mutate: func(value *ExportCheckpoint) { value.MapHash = "ABC" }, wantErr: "map hash"},
		{name: "invalid requester", mutate: func(value *ExportCheckpoint) { value.RequestedBy = "invalid" }, wantErr: "actor id"},
		{name: "pending terminal metadata", mutate: func(value *ExportCheckpoint) { value.ArtifactHash = strings.Repeat("b", 64) }, wantErr: "pending"},
		{name: "unknown status", mutate: func(value *ExportCheckpoint) { value.Status = "complete" }, wantErr: "status"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := exportCheckpointFixture()
			test.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, test.wantErr)
			}
		})
	}
}

func TestExportCheckpointRejectsInvalidCompletion(t *testing.T) {
	t.Parallel()

	pending := exportCheckpointFixture()
	tests := []struct {
		name       string
		completion ExportCheckpointCompletion
		wantErr    string
	}{
		{name: "pending completion", completion: ExportCheckpointCompletion{Status: ExportCheckpointPending}, wantErr: "terminal"},
		{name: "accepted without artifact", completion: ExportCheckpointCompletion{Status: ExportCheckpointAccepted, Verifier: "mcp", VerifierVersion: "1", CompletedAt: time.Unix(20, 0).UTC()}, wantErr: "artifact hash"},
		{name: "rejected without diagnostic", completion: ExportCheckpointCompletion{Status: ExportCheckpointRejected, Verifier: "mcp", VerifierVersion: "1", CompletedAt: time.Unix(20, 0).UTC()}, wantErr: "diagnostic"},
		{name: "oversized diagnostic", completion: ExportCheckpointCompletion{Status: ExportCheckpointRejected, Verifier: "mcp", VerifierVersion: "1", DiagnosticCode: strings.Repeat("x", MaxCheckpointTextBytes+1), CompletedAt: time.Unix(20, 0).UTC()}, wantErr: "diagnostic"},
		{name: "completion before creation", completion: ExportCheckpointCompletion{Status: ExportCheckpointRejected, Verifier: "mcp", VerifierVersion: "1", DiagnosticCode: "failed", CompletedAt: time.Unix(5, 0).UTC()}, wantErr: "before"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := pending.Complete(test.completion); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Complete() error = %v, want error containing %q", err, test.wantErr)
			}
		})
	}
}

func exportCheckpointFixture() ExportCheckpoint {
	return ExportCheckpoint{
		CheckpointID:   CheckpointID("01890f3e-7b5c-7abc-8def-0123456789ef"),
		IdempotencyKey: "01890f3e-7b5c-7abc-8def-0123456789ee",
		DocumentID:     testDocumentID,
		SessionID:      "session-1",
		Revision:       4,
		MapHash:        strings.Repeat("a", 64),
		RequestedBy:    ActorID("01890f3e-7b5c-7abc-8def-0123456789ba"),
		CreatedAt:      time.Unix(10, 0).UTC(),
		Status:         ExportCheckpointPending,
	}
}
