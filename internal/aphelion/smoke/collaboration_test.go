package smoke

import (
	"context"
	"strings"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/model"
)

func TestRunCollaborationRecordsTwoClientConvergenceAndShutdown(t *testing.T) {
	t.Parallel()

	snapshot := collaborationSmokeSnapshot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	report := RunCollaboration(ctx, snapshot)

	if !report.OK() {
		t.Fatalf("collaboration smoke failed: %#v", report)
	}
	if !report.SessionIDPresent || !report.DistinctActors {
		t.Fatalf("session identity evidence = present %t distinct actors %t", report.SessionIDPresent, report.DistinctActors)
	}
	if len(report.AcceptedRevisions) != 2 || report.AcceptedRevisions[0] != 1 || report.AcceptedRevisions[1] != 2 {
		t.Fatalf("accepted revisions = %v, want [1 2]", report.AcceptedRevisions)
	}
	if report.ClientAHash == "" || report.ClientAHash != report.ClientBHash {
		t.Fatalf("client hashes = %q and %q", report.ClientAHash, report.ClientBHash)
	}
}

func TestRunCollaborationProducesReproducibleFinalHash(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first := RunCollaboration(ctx, collaborationSmokeSnapshot(t))
	second := RunCollaboration(ctx, collaborationSmokeSnapshot(t))
	if !first.OK() || !second.OK() {
		t.Fatalf("collaboration smoke runs failed: first=%#v second=%#v", first, second)
	}
	if first.ClientAHash != second.ClientAHash {
		t.Fatalf("final hashes differ across runs: %q and %q", first.ClientAHash, second.ClientAHash)
	}
}

func collaborationSmokeSnapshot(t *testing.T) model.Snapshot {
	t.Helper()
	documentID, err := model.NewDocumentID()
	if err != nil {
		t.Fatal(err)
	}
	return model.Snapshot{
		ProtocolVersion: model.ProtocolVersion,
		SchemaVersion:   model.SchemaVersion,
		DocumentID:      documentID,
		EnvironmentHash: strings.Repeat("a", 64),
		MaxX:            2,
		MaxY:            1,
		MaxZ:            1,
	}
}
