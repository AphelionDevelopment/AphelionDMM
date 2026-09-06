package editing

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"sdmm/internal/aphelion/collab/model"
)

func resizeFixture(t testing.TB) model.Snapshot {
	t.Helper()
	id, err := model.NewDocumentID()
	if err != nil {
		t.Fatal(err)
	}
	stable, err := model.NewStableID()
	if err != nil {
		t.Fatal(err)
	}
	return model.Snapshot{ProtocolVersion: model.ProtocolVersion, SchemaVersion: model.SchemaVersion, DocumentID: id, Revision: 7, EnvironmentHash: strings.Repeat("a", 64), MaxX: 2, MaxY: 2, MaxZ: 2,
		Tiles: []model.Tile{{Coord: model.Coord{X: 1, Y: 1, Z: 1}, State: model.TileState{Prefabs: []model.PrefabState{{StableID: stable, Path: "/missing/type", Vars: map[string]string{"opaque": `list("unchanged")`}}}}}}}
}

func TestResizePlansIsolatedStateAndFreshDocument(t *testing.T) {
	source := resizeFixture(t)
	before := model.CloneSnapshot(source)
	result, err := Resize(source, 3, 1, 1, "/turf/default", "/area/default")
	if err != nil {
		t.Fatal(err)
	}
	if result.DocumentID == source.DocumentID || result.Revision != 0 || len(result.Tiles) != 2 {
		t.Fatal("resize did not create a distinct local document with only retained/new tiles")
	}
	if result.Tiles[0].State.Prefabs[0].StableID != source.Tiles[0].State.Prefabs[0].StableID {
		t.Fatal("retained ID changed")
	}
	added := result.Tiles[1]
	if added.Coord != (model.Coord{X: 3, Y: 1, Z: 1}) || len(added.State.Prefabs) != 2 || added.State.Prefabs[0].Path != "/turf/default" || added.State.Prefabs[1].Path != "/area/default" {
		t.Fatal("new cell did not receive the loaded world's defaults")
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.Tiles[0].State.Prefabs[0].Vars["opaque"] = "mutated candidate"
	if !reflect.DeepEqual(source, before) {
		t.Fatal("candidate aliases the source")
	}
}

func TestResizeRejectsDimensionsAndDefaults(t *testing.T) {
	source := resizeFixture(t)
	for _, size := range [][3]int{{0, 1, 1}, {-1, 1, 1}, {math.MaxInt, math.MaxInt, math.MaxInt}, {4096, 4096, 2}} {
		if _, err := Resize(source, size[0], size[1], size[2], "/turf/default", "/area/default"); err == nil {
			t.Fatal("unsafe dimensions accepted")
		}
	}
	for _, paths := range [][2]string{{"", "/area/default"}, {"null", "/area/default"}, {"/turf/default", "BAD_AREA"}} {
		if _, err := Resize(source, 3, 2, 2, paths[0], paths[1]); err == nil {
			t.Fatal("missing or opaque default path accepted for new cells")
		}
	}
	if _, err := Resize(source, 1, 1, 1, "", ""); err != nil {
		t.Fatal("shrinking requires no new base instances", err)
	}
}
