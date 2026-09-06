// Package search provides map-query operations independent of UI widgets.
package search

import (
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmminstance"
)

// ByPrefabIDs scans the map once, retaining the caller's prefab grouping order
// and each group's original tile/instance order. Results refer to current map
// instances, just like the editor's single-prefab query; this is not a cache.
func ByPrefabIDs(m *dmmap.Dmm, ids []uint64) (result []*dmminstance.Instance) {
	if m == nil || len(ids) == 0 {
		return nil
	}
	if len(ids) == 1 {
		for _, tile := range m.Tiles {
			for _, instance := range tile.Instances() {
				if instance.Prefab().Id() == ids[0] {
					result = append(result, instance)
				}
			}
		}
		return
	}
	groups := make(map[uint64][]*dmminstance.Instance, len(ids))
	for _, id := range ids {
		groups[id] = nil
	}
	for _, tile := range m.Tiles {
		for _, instance := range tile.Instances() {
			id := instance.Prefab().Id()
			if group, exists := groups[id]; exists {
				groups[id] = append(group, instance)
			}
		}
	}
	count := 0
	for _, id := range ids {
		count += len(groups[id])
	}
	if count == 0 {
		return nil
	}
	result = make([]*dmminstance.Instance, 0, count)
	for _, id := range ids {
		result = append(result, groups[id]...)
	}
	return
}
