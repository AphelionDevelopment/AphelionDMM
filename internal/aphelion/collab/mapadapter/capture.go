package mapadapter

import (
	"fmt"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/dmapi/dmmap"
)

func CaptureTile(tile *dmmap.Tile) (model.TileState, error) {
	if tile == nil {
		return model.TileState{}, fmt.Errorf("capture tile: tile is nil")
	}
	state := model.TileState{Prefabs: make([]model.PrefabState, len(tile.Instances()))}
	for index, instance := range tile.Instances() {
		if instance == nil || instance.Prefab() == nil || instance.Prefab().Vars() == nil {
			return model.TileState{}, fmt.Errorf("capture tile: prefab %d is nil", index)
		}
		prefab := instance.Prefab()
		stableID := model.StableID(instance.StableID())
		if stableID == "" {
			generated, err := model.NewStableID()
			if err != nil {
				return model.TileState{}, fmt.Errorf("capture tile: assign stable id: %w", err)
			}
			stableID = generated
			instance.SetStableID(string(generated))
		}
		if err := stableID.Validate(); err != nil {
			return model.TileState{}, fmt.Errorf("capture tile: %w", err)
		}
		state.Prefabs[index] = model.PrefabState{
			StableID: stableID,
			Path:     prefab.Path(),
			Vars:     make(map[string]string, prefab.Vars().Len()),
		}
		for _, name := range prefab.Vars().Iterate() {
			value, exists := prefab.Vars().Value(name)
			if !exists {
				return model.TileState{}, fmt.Errorf("capture tile: variable %q on %q has no value", name, prefab.Path())
			}
			state.Prefabs[index].Vars[name] = value
		}
	}
	return state, nil
}
