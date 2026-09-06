// APHELION EDIT ADDITION START - SEARCH QUERY LIFECYCLE
package cpsearch

import (
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	mapsearch "sdmm/internal/aphelion/search"
	"sdmm/internal/dmapi/dmmap"
)

func (s *Search) searchCurrentMap() {
	s.Free()
	ed := s.currentEditor()
	if ed == nil {
		return
	}
	version, ready := ed.MapViewVersion()
	if !ready {
		return
	}
	s.resultEditor, s.resultVersion, s.resultReady = ed, version, true
	if s.prefabId == "" {
		return
	}
	log.Print("searching for:", s.prefabId)
	if strings.HasPrefix(s.prefabId, "/") {
		prefabs := dmmap.PrefabStorage.GetAllByPath(s.prefabId)
		ids := make([]uint64, len(prefabs))
		for i, prefab := range prefabs {
			ids[i] = prefab.Id()
		}
		s.resultsAll = mapsearch.ByPrefabIDs(ed.Dmm(), ids)
	} else {
		id, err := strconv.ParseUint(s.prefabId, 10, 64)
		if err != nil {
			return
		}
		s.resultsAll = mapsearch.ByPrefabIDs(ed.Dmm(), []uint64{id})
	}
	log.Print("found search results:", len(s.resultsAll))
}

func (s *Search) resetResultNavigation() {
	s.selectedResultIdx, s.focusedResultIdx, s.lastFocusedResultIdx = -1, -1, -1
	s.resultGeneration++
}

// APHELION EDIT ADDITION END
