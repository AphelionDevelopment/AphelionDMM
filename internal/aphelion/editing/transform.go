package editing

import (
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/util"
)

// Transform is a fully validated plan. Callers capture every before-state before
// installing any tile, then submit the whole plan as one authoritative operation.
type Transform struct {
	Bounds util.Bounds
	Tiles  []dmmap.Tile
}
