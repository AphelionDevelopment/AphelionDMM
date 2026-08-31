package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const MaxCheckpointTextBytes = 128

type ExportCheckpointStatus string

const (
	ExportCheckpointPending  ExportCheckpointStatus = "pending"
	ExportCheckpointAccepted ExportCheckpointStatus = "accepted"
	ExportCheckpointRejected ExportCheckpointStatus = "rejected"
)

var checkpointCodePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

type ExportCheckpoint struct {
	CheckpointID    CheckpointID           `json:"checkpoint_id"`
	IdempotencyKey  string                 `json:"idempotency_key"`
	DocumentID      DocumentID             `json:"document_id"`
	SessionID       string                 `json:"session_id"`
	Revision        Revision               `json:"revision"`
	MapHash         string                 `json:"map_hash"`
	RequestedBy     ActorID                `json:"requested_by"`
	CreatedAt       time.Time              `json:"created_at"`
	Status          ExportCheckpointStatus `json:"status"`
	ArtifactHash    string                 `json:"artifact_hash,omitempty"`
	Verifier        string                 `json:"verifier,omitempty"`
	VerifierVersion string                 `json:"verifier_version,omitempty"`
	DiagnosticCode  string                 `json:"diagnostic_code,omitempty"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
}

type ExportCheckpointCompletion struct {
	Status          ExportCheckpointStatus
	ArtifactHash    string
	Verifier        string
	VerifierVersion string
	DiagnosticCode  string
	CompletedAt     time.Time
}

func CloneExportCheckpoint(checkpoint ExportCheckpoint) ExportCheckpoint {
	if checkpoint.CompletedAt != nil {
		completedAt := *checkpoint.CompletedAt
		checkpoint.CompletedAt = &completedAt
	}
	return checkpoint
}

func (checkpoint ExportCheckpoint) Validate() error {
	if err := checkpoint.CheckpointID.Validate(); err != nil {
		return err
	}
	if err := validateCheckpointText("idempotency key", checkpoint.IdempotencyKey, false); err != nil {
		return err
	}
	if err := checkpoint.DocumentID.Validate(); err != nil {
		return err
	}
	if err := validateCheckpointText("session id", checkpoint.SessionID, false); err != nil {
		return err
	}
	if err := ValidateSHA256("map hash", checkpoint.MapHash); err != nil {
		return err
	}
	if err := checkpoint.RequestedBy.Validate(); err != nil {
		return err
	}
	if checkpoint.CreatedAt.IsZero() {
		return fmt.Errorf("checkpoint creation time is required")
	}
	switch checkpoint.Status {
	case ExportCheckpointPending:
		if checkpoint.ArtifactHash != "" || checkpoint.Verifier != "" || checkpoint.VerifierVersion != "" || checkpoint.DiagnosticCode != "" || checkpoint.CompletedAt != nil {
			return fmt.Errorf("pending checkpoint contains terminal metadata")
		}
		return nil
	case ExportCheckpointAccepted:
		if err := ValidateSHA256("artifact hash", checkpoint.ArtifactHash); err != nil {
			return err
		}
	case ExportCheckpointRejected:
		if checkpoint.ArtifactHash != "" {
			return fmt.Errorf("rejected checkpoint must not contain an artifact hash")
		}
		if err := validateDiagnosticCode(checkpoint.DiagnosticCode); err != nil {
			return err
		}
	default:
		return fmt.Errorf("checkpoint status %q is invalid", checkpoint.Status)
	}
	if err := validateCheckpointText("verifier", checkpoint.Verifier, false); err != nil {
		return err
	}
	if err := validateCheckpointText("verifier version", checkpoint.VerifierVersion, false); err != nil {
		return err
	}
	if checkpoint.CompletedAt == nil {
		return fmt.Errorf("terminal checkpoint completion time is required")
	}
	if checkpoint.CompletedAt.Before(checkpoint.CreatedAt) {
		return fmt.Errorf("checkpoint completion time is before creation time")
	}
	return nil
}

func (checkpoint ExportCheckpoint) Complete(completion ExportCheckpointCompletion) (ExportCheckpoint, error) {
	if err := checkpoint.Validate(); err != nil {
		return ExportCheckpoint{}, err
	}
	if checkpoint.Status != ExportCheckpointPending {
		return ExportCheckpoint{}, fmt.Errorf("checkpoint status %q is already terminal", checkpoint.Status)
	}
	if completion.Status != ExportCheckpointAccepted && completion.Status != ExportCheckpointRejected {
		return ExportCheckpoint{}, fmt.Errorf("checkpoint completion status %q is not terminal", completion.Status)
	}
	completedAt := completion.CompletedAt.UTC()
	checkpoint.Status = completion.Status
	checkpoint.ArtifactHash = completion.ArtifactHash
	checkpoint.Verifier = completion.Verifier
	checkpoint.VerifierVersion = completion.VerifierVersion
	checkpoint.DiagnosticCode = completion.DiagnosticCode
	checkpoint.CompletedAt = &completedAt
	if err := checkpoint.Validate(); err != nil {
		return ExportCheckpoint{}, err
	}
	return checkpoint, nil
}

func validateCheckpointText(name, value string, allowEmpty bool) error {
	if !allowEmpty && strings.TrimSpace(value) == "" {
		return fmt.Errorf("checkpoint %s is required", name)
	}
	if len(value) > MaxCheckpointTextBytes {
		return fmt.Errorf("checkpoint %s exceeds %d bytes", name, MaxCheckpointTextBytes)
	}
	return nil
}

func validateDiagnosticCode(value string) error {
	if err := validateCheckpointText("diagnostic code", value, false); err != nil {
		return err
	}
	if !checkpointCodePattern.MatchString(value) {
		return fmt.Errorf("checkpoint diagnostic code is invalid")
	}
	return nil
}
