package editing

import (
	"fmt"
	"strings"

	"sdmm/internal/aphelion/collab/model"
)

// Resize prepares an isolated local maintenance document. Retained cells keep
// their explicit contents/IDs, including sparse empty cells and unknown types.
// New cells receive turf then area, matching the inherited resize convention.
// Undo/redo must retain this document rather than generate new IDs again.
func Resize(source model.Snapshot, x, y, z int, turf, area string) (model.Snapshot, error) {
	target := source
	target.MaxX, target.MaxY, target.MaxZ = x, y, z
	cellCount, err := target.CellCount()
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("resize map: %w", err)
	}
	if err := source.Validate(); err != nil {
		return model.Snapshot{}, fmt.Errorf("resize source: %w", err)
	}
	growing := x > source.MaxX || y > source.MaxY || z > source.MaxZ
	if growing {
		for _, path := range []string{turf, area} {
			if len(path) < 2 || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, " \t\r\n{}();\"'") {
				return model.Snapshot{}, fmt.Errorf("resize map: default turf and area must be explicit type paths")
			}
		}
	}
	target.DocumentID, err = model.NewDocumentID()
	if err != nil {
		return model.Snapshot{}, err
	}
	target.Revision = 0
	target.Tiles = make([]model.Tile, 0, min(cellCount, len(source.Tiles)))
	for _, tile := range source.Tiles {
		if target.Contains(tile.Coord) {
			target.Tiles = append(target.Tiles, model.Tile{Coord: tile.Coord, State: model.CloneTileState(tile.State)})
		}
	}
	if growing {
		for dz := 1; dz <= z; dz++ {
			for dy := 1; dy <= y; dy++ {
				// Existing rows contain no new X cells until source.MaxX+1.
				firstX := 1
				if dz <= source.MaxZ && dy <= source.MaxY {
					firstX = source.MaxX + 1
				}
				for dx := firstX; dx <= x; dx++ {
					state := model.TileState{Prefabs: make([]model.PrefabState, 2)}
					for index, path := range []string{turf, area} {
						id, err := model.NewStableID()
						if err != nil {
							return model.Snapshot{}, err
						}
						state.Prefabs[index] = model.PrefabState{StableID: id, Path: path}
					}
					target.Tiles = append(target.Tiles, model.Tile{Coord: model.Coord{X: dx, Y: dy, Z: dz}, State: state})
				}
			}
		}
	}
	return target, nil
}
