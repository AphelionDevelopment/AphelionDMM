package store

import (
	"sdmm/internal/aphelion/collab/model"
	"testing"
	"time"
)

func TestCheckpointComparisonPreservesInstantsAndFields(t *testing.T) {
	completed := time.Unix(5, 123).UTC()
	left := model.ExportCheckpoint{CreatedAt: time.Unix(3, 123).UTC(), CompletedAt: &completed, Status: model.ExportCheckpointAccepted}
	right := model.CloneExportCheckpoint(left)
	right.CreatedAt = right.CreatedAt.In(time.FixedZone("fixture", 3600))
	shifted := right.CompletedAt.In(time.FixedZone("fixture", 3600))
	right.CompletedAt = &shifted
	if !sameCheckpointState(left, right) {
		t.Fatal("same instants compared unequal")
	}
	right.CreatedAt = right.CreatedAt.Add(time.Nanosecond)
	if sameCheckpointState(left, right) {
		t.Fatal("different instant compared equal")
	}
	right.CreatedAt = left.CreatedAt
	right.ArtifactHash = "changed"
	if sameCheckpointState(left, right) {
		t.Fatal("different content compared equal")
	}
}
