package model

func (state TileState) Equal(other TileState) bool {
	if len(state.Prefabs) != len(other.Prefabs) {
		return false
	}
	for index, prefab := range state.Prefabs {
		otherPrefab := other.Prefabs[index]
		if prefab.StableID != otherPrefab.StableID || prefab.Path != otherPrefab.Path || len(prefab.Vars) != len(otherPrefab.Vars) {
			return false
		}
		for name, value := range prefab.Vars {
			if otherValue, exists := otherPrefab.Vars[name]; !exists || otherValue != value {
				return false
			}
		}
	}
	return true
}

func CloneTileState(state TileState) TileState {
	if state.Prefabs == nil {
		return TileState{}
	}
	clone := TileState{Prefabs: make([]PrefabState, len(state.Prefabs))}
	for index, prefab := range state.Prefabs {
		clone.Prefabs[index] = PrefabState{
			StableID: prefab.StableID,
			Path:     prefab.Path,
		}
		if prefab.Vars != nil {
			clone.Prefabs[index].Vars = make(map[string]string, len(prefab.Vars))
			for name, value := range prefab.Vars {
				clone.Prefabs[index].Vars[name] = value
			}
		}
	}
	return clone
}

func CloneSnapshot(snapshot Snapshot) Snapshot {
	clone := snapshot
	clone.Tiles = make([]Tile, len(snapshot.Tiles))
	for index, tile := range snapshot.Tiles {
		clone.Tiles[index] = Tile{
			Coord: tile.Coord,
			State: CloneTileState(tile.State),
		}
	}
	return clone
}

func CloneOperation(operation Operation) Operation {
	clone := operation
	clone.Changes = make([]TileChange, len(operation.Changes))
	for index, change := range operation.Changes {
		clone.Changes[index] = TileChange{
			Coord:  change.Coord,
			Before: CloneTileState(change.Before),
			After:  CloneTileState(change.After),
		}
	}
	if operation.InverseOf != nil {
		inverseOf := *operation.InverseOf
		clone.InverseOf = &inverseOf
	}
	return clone
}

func CloneAcceptedOperation(operation AcceptedOperation) AcceptedOperation {
	return AcceptedOperation{
		Operation:  CloneOperation(operation.Operation),
		Revision:   operation.Revision,
		AcceptedAt: operation.AcceptedAt,
	}
}
