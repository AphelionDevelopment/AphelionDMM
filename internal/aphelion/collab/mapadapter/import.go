package mapadapter

import (
	"fmt"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/util"
)

func Import(source *dmmap.Dmm, documentID model.DocumentID, environmentHash string) (model.Snapshot, error) {
	return importSnapshot(source, model.Snapshot{
		ProtocolVersion: model.ProtocolVersion,
		SchemaVersion:   model.SchemaVersion,
		DocumentID:      documentID,
		EnvironmentHash: environmentHash,
	})
}

func Reimport(source *dmmap.Dmm, metadata model.Snapshot) (model.Snapshot, error) {
	if _, err := metadata.Hash(); err != nil {
		return model.Snapshot{}, fmt.Errorf("validate collaboration metadata: %w", err)
	}
	return importSnapshot(source, metadata)
}

func importSnapshot(source *dmmap.Dmm, metadata model.Snapshot) (model.Snapshot, error) {
	if source == nil {
		return model.Snapshot{}, fmt.Errorf("import map: source is nil")
	}
	if err := metadata.DocumentID.Validate(); err != nil {
		return model.Snapshot{}, err
	}
	if err := model.ValidateSHA256("environment hash", metadata.EnvironmentHash); err != nil {
		return model.Snapshot{}, err
	}
	if source.MaxX <= 0 || source.MaxY <= 0 || source.MaxZ <= 0 {
		return model.Snapshot{}, fmt.Errorf("import map: dimensions must be positive")
	}
	expectedTiles := source.MaxX * source.MaxY * source.MaxZ
	if len(source.Tiles) != expectedTiles {
		return model.Snapshot{}, fmt.Errorf("import map: tile count is %d, want %d", len(source.Tiles), expectedTiles)
	}
	useMetadata := len(metadata.Tiles) != 0
	if useMetadata && (metadata.MaxX != source.MaxX || metadata.MaxY != source.MaxY || metadata.MaxZ != source.MaxZ) {
		return model.Snapshot{}, fmt.Errorf("reimport map: dimensions differ from collaboration metadata")
	}
	metadataStates := statesByCoord(metadata)

	snapshot := model.Snapshot{
		ProtocolVersion: model.ProtocolVersion,
		SchemaVersion:   model.SchemaVersion,
		DocumentID:      metadata.DocumentID,
		Revision:        metadata.Revision,
		EnvironmentHash: metadata.EnvironmentHash,
		MaxX:            source.MaxX,
		MaxY:            source.MaxY,
		MaxZ:            source.MaxZ,
		Tiles:           make([]model.Tile, 0, expectedTiles),
	}
	for z := 1; z <= source.MaxZ; z++ {
		for y := 1; y <= source.MaxY; y++ {
			for x := 1; x <= source.MaxX; x++ {
				coord := model.Coord{X: x, Y: y, Z: z}
				tile := source.GetTile(util.Point{X: x, Y: y, Z: z})
				if tile == nil {
					return model.Snapshot{}, fmt.Errorf("import map: tile (%d,%d,%d) is nil", x, y, z)
				}
				state := model.TileState{Prefabs: make([]model.PrefabState, len(tile.Instances()))}
				for index, instance := range tile.Instances() {
					prefab := instance.Prefab()
					state.Prefabs[index] = model.PrefabState{
						Path: prefab.Path(),
						Vars: make(map[string]string, prefab.Vars().Len()),
					}
					for _, name := range prefab.Vars().Iterate() {
						value, exists := prefab.Vars().Value(name)
						if !exists {
							return model.Snapshot{}, fmt.Errorf("import map: variable %q on %q has no value", name, prefab.Path())
						}
						state.Prefabs[index].Vars[name] = value
					}

					stableID := model.StableID(instance.StableID())
					if useMetadata {
						metadataState, exists := metadataStates[coord]
						if !exists || index >= len(metadataState.Prefabs) {
							return model.Snapshot{}, fmt.Errorf("reimport map: prefab layout differs at (%d,%d,%d)", x, y, z)
						}
						metadataPrefab := metadataState.Prefabs[index]
						candidate := state.Prefabs[index]
						candidate.StableID = metadataPrefab.StableID
						if !(model.TileState{Prefabs: []model.PrefabState{candidate}}).Equal(model.TileState{Prefabs: []model.PrefabState{metadataPrefab}}) {
							return model.Snapshot{}, fmt.Errorf("reimport map: prefab content differs at (%d,%d,%d) index %d", x, y, z, index)
						}
						stableID = metadataPrefab.StableID
					} else if stableID == "" {
						generated, err := model.NewStableID()
						if err != nil {
							return model.Snapshot{}, fmt.Errorf("import map: assign stable id: %w", err)
						}
						stableID = generated
					}
					if err := stableID.Validate(); err != nil {
						return model.Snapshot{}, fmt.Errorf("import map: %w", err)
					}
					state.Prefabs[index].StableID = stableID
					instance.SetStableID(string(stableID))
				}
				if useMetadata {
					metadataState := metadataStates[coord]
					if len(state.Prefabs) != len(metadataState.Prefabs) {
						return model.Snapshot{}, fmt.Errorf("reimport map: prefab count differs at (%d,%d,%d)", x, y, z)
					}
				}
				snapshot.Tiles = append(snapshot.Tiles, model.Tile{Coord: coord, State: state})
			}
		}
	}
	if _, err := snapshot.Hash(); err != nil {
		return model.Snapshot{}, fmt.Errorf("import map: validate snapshot: %w", err)
	}
	return snapshot, nil
}

func statesByCoord(snapshot model.Snapshot) map[model.Coord]model.TileState {
	states := make(map[model.Coord]model.TileState, len(snapshot.Tiles))
	for _, tile := range snapshot.Tiles {
		states[tile.Coord] = tile.State
	}
	return states
}
