package client

import "sdmm/internal/aphelion/collab/model"

// visibleProjection owns one deep copy. Indexes live only for this projection
// call, so neither callers nor concurrent readers can observe partial edits.
func visibleProjection(acknowledged model.Snapshot, pending []model.Operation) model.Snapshot {
	visible := model.CloneSnapshot(acknowledged)
	if len(pending) == 0 {
		return visible
	}
	if err := visible.Validate(); err != nil {
		// Projection is a public value. Preserve legacy behavior for malformed
		// baselines, including an operation that repairs the invalid tile state.
		for _, operation := range pending {
			if next, err := applyOperation(visible, operation); err == nil {
				visible = next
			}
		}
		return visible
	}
	index := projectionIndex{snapshot: &visible, tiles: make(map[model.Coord]int, len(visible.Tiles)), owners: make(map[model.StableID]model.Coord)}
	for i, tile := range visible.Tiles {
		index.tiles[tile.Coord] = i
		for _, prefab := range tile.State.Prefabs {
			index.owners[prefab.StableID] = tile.Coord
		}
	}
	for _, operation := range pending {
		index.apply(operation)
	}
	return visible
}

type projectionIndex struct {
	snapshot *model.Snapshot
	tiles    map[model.Coord]int
	owners   map[model.StableID]model.Coord
}

// apply validates the complete batch before mutation, including IDs moving
// between tiles in either change order. Metadata/dimensions are immutable and
// validated once; these checks maintain Snapshot.Validate's coordinate and ID
// invariants incrementally. Visible projection needs no canonical digest here.
// Submitted request headers/preconditions are never rewritten.
func (index *projectionIndex) apply(operation model.Operation) {
	affected := make(map[model.Coord]struct{}, len(operation.Changes))
	for _, change := range operation.Changes {
		if _, duplicate := affected[change.Coord]; duplicate || !index.snapshot.Contains(change.Coord) {
			return
		}
		affected[change.Coord] = struct{}{}
		current := model.TileState{}
		if i, exists := index.tiles[change.Coord]; exists {
			current = index.snapshot.Tiles[i].State
		}
		if !current.Equal(change.Before) {
			return
		}
	}
	afterIDs := make(map[model.StableID]struct{})
	for _, change := range operation.Changes {
		for _, prefab := range change.After.Prefabs {
			if err := prefab.StableID.Validate(); err != nil {
				return
			}
			if _, duplicate := afterIDs[prefab.StableID]; duplicate {
				return
			}
			afterIDs[prefab.StableID] = struct{}{}
			if owner, exists := index.owners[prefab.StableID]; exists {
				if _, changingOwner := affected[owner]; !changingOwner {
					return
				}
			}
		}
	}
	// Remove all old owners before assigning any new ones (including swaps).
	for _, change := range operation.Changes {
		if i, exists := index.tiles[change.Coord]; exists {
			for _, prefab := range index.snapshot.Tiles[i].State.Prefabs {
				delete(index.owners, prefab.StableID)
			}
		}
	}
	for _, change := range operation.Changes {
		after := model.CloneTileState(change.After)
		if i, exists := index.tiles[change.Coord]; exists {
			index.snapshot.Tiles[i].State = after
		} else {
			index.tiles[change.Coord] = len(index.snapshot.Tiles)
			index.snapshot.Tiles = append(index.snapshot.Tiles, model.Tile{Coord: change.Coord, State: after})
		}
		for _, prefab := range after.Prefabs {
			index.owners[prefab.StableID] = change.Coord
		}
	}
}
