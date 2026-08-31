package model

import "fmt"

const (
	MaxMapDimension = 4096
	MaxMapCells     = 16_777_216
)

// CellCount returns the bounded number of cells represented by the snapshot dimensions.
func (snapshot Snapshot) CellCount() (int, error) {
	if snapshot.MaxX <= 0 || snapshot.MaxY <= 0 || snapshot.MaxZ <= 0 {
		return 0, fmt.Errorf("dimensions must be positive: got (%d,%d,%d)", snapshot.MaxX, snapshot.MaxY, snapshot.MaxZ)
	}
	if snapshot.MaxX > MaxMapDimension || snapshot.MaxY > MaxMapDimension || snapshot.MaxZ > MaxMapDimension {
		return 0, fmt.Errorf("dimensions (%d,%d,%d) exceed maximum %d", snapshot.MaxX, snapshot.MaxY, snapshot.MaxZ, MaxMapDimension)
	}
	if snapshot.MaxX > MaxMapCells/snapshot.MaxY {
		return 0, fmt.Errorf("dimension cell count exceeds maximum %d", MaxMapCells)
	}
	xy := snapshot.MaxX * snapshot.MaxY
	if xy > MaxMapCells/snapshot.MaxZ {
		return 0, fmt.Errorf("dimension cell count exceeds maximum %d", MaxMapCells)
	}
	return xy * snapshot.MaxZ, nil
}
