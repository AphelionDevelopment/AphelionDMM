package store

import (
	"reflect"
	"sdmm/internal/aphelion/collab/model"
)

// SQL timestamptz preserves the instant, not the original Go time.Location.
// All non-time fields, nil completion state, and timestamp precision still match.
func sameCheckpointState(left, right model.ExportCheckpoint) bool {
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return false
	}
	if (left.CompletedAt == nil) != (right.CompletedAt == nil) {
		return false
	}
	if left.CompletedAt != nil && !left.CompletedAt.Equal(*right.CompletedAt) {
		return false
	}
	left.CreatedAt, left.CompletedAt = right.CreatedAt, right.CompletedAt
	return reflect.DeepEqual(left, right)
}
