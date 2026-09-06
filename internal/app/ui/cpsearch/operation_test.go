package cpsearch

import (
	"context"
	"reflect"
	"testing"

	"github.com/SpaiR/imgui-go"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/canvas"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/editor"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmsnap"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

type searchOperationApp struct {
	*searchTestApp
	environment *dmenv.Dme
	selected    *dmmprefab.Prefab
	errors      []error
}

func (a *searchOperationApp) LoadedEnvironment() *dmenv.Dme { return a.environment }
func (a *searchOperationApp) SelectedPrefab() (*dmmprefab.Prefab, bool) {
	return a.selected, a.selected != nil
}
func (a *searchOperationApp) ReportCollaborationError(_ string, err error) {
	a.errors = append(a.errors, err)
}

// Authority, map operations and history are real. Rendering is not exercised:
// the editor's scheduled bucket updates remain in the window queue, as in the
// editor's non-GL operation tests. Real workspace rendering is a separate gate.
type searchOperationMap struct{ snapshot *dmmsnap.DmmSnap }

func (*searchOperationMap) ActiveLevel() int                                  { return 1 }
func (*searchOperationMap) SetActiveLevel(int)                                {}
func (m *searchOperationMap) Snapshot() *dmmsnap.DmmSnap                      { return m.snapshot }
func (*searchOperationMap) Size() imgui.Vec2                                  { return imgui.Vec2{} }
func (*searchOperationMap) Canvas() *canvas.Canvas                            { return &canvas.Canvas{} }
func (*searchOperationMap) CanvasState() *canvas.State                        { return nil }
func (*searchOperationMap) CanvasControl() *canvas.Control                    { return nil }
func (*searchOperationMap) CanvasOverlay() *canvas.Overlay                    { return nil }
func (*searchOperationMap) PushAreaHover(util.Bounds, util.Color, util.Color) {}
func (*searchOperationMap) OnMapSizeChange()                                  {}

func searchOperationFixture(t *testing.T) (*Search, *searchOperationApp) {
	t.Helper()
	s := searchFixture(t, 3, 1)
	objects := make(map[string]*dmenv.Object)
	for _, path := range []string{"/world", "/area/test", "/turf/test", "/obj/search", "/obj/replacement"} {
		vars := &dmvars.MutableVariables{}
		if path == "/world" {
			vars.Put("area", "/area/test")
			vars.Put("turf", "/turf/test")
		}
		objects[path] = &dmenv.Object{Path: path, Vars: vars.ToImmutable()}
	}
	a := &searchOperationApp{searchTestApp: s.app.(*searchTestApp), environment: &dmenv.Dme{Objects: objects}}
	dmmap.Init(a.environment)
	t.Cleanup(dmmap.Free)
	m := a.current.Dmm()
	m.Path.Absolute = "search-operation"
	a.commands.SetStack(m.Path.Absolute)
	for _, tile := range m.Tiles {
		tile.InstancesRegenerate()
	}
	a.current = editor.New(a, &searchOperationMap{snapshot: dmmsnap.New(m)}, m)
	t.Cleanup(a.current.Close)
	a.selected = dmmap.PrefabStorage.Initial("/obj/replacement")
	s.app = a
	s.SearchByPath("/obj/search")
	return s, a
}

func searchAuthority(t *testing.T, e *editor.Editor) (model.Snapshot, string) {
	t.Helper()
	snapshot, err := e.SaveSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := snapshot.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, hash
}

func TestSearchMutationsUseAuthorityAndUndo(t *testing.T) {
	for _, test := range []struct {
		name      string
		action    func(*Search)
		remaining int
		replaced  int
	}{
		{"delete row", func(s *Search) { s.deleteInstance(1) }, 2, 0},
		{"replace row", func(s *Search) { s.replaceInstance(1) }, 2, 1},
		{"delete all", (*Search).doDeleteAll, 0, 0},
		{"replace all", (*Search).doReplaceAll, 0, 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, a := searchOperationFixture(t)
			e := a.current
			_, before := searchAuthority(t, e)
			test.action(s)
			snapshot, after := searchAuthority(t, e)
			remaining, replaced := 0, 0
			for _, tile := range snapshot.Tiles {
				for _, prefab := range tile.State.Prefabs {
					switch prefab.Path {
					case "/obj/search":
						remaining++
					case "/obj/replacement":
						replaced++
					}
				}
			}
			if snapshot.Revision != 1 || remaining != test.remaining || replaced != test.replaced || len(s.results()) != test.remaining || len(a.errors) != 0 {
				t.Fatalf("action did not publish exactly one expected operation: revision=%d remaining=%d replaced=%d results=%d errors=%v", snapshot.Revision, remaining, replaced, len(s.results()), a.errors)
			}
			a.commands.UndoV(e.Dmm().Path.Absolute)
			_, undone := searchAuthority(t, e)
			if undone != before {
				t.Fatal("search undo did not restore the exact map hash")
			}
			a.commands.RedoV(e.Dmm().Path.Absolute)
			_, redone := searchAuthority(t, e)
			if redone != after {
				t.Fatal("search redo did not restore the action result")
			}
		})
	}
}

func TestSearchMutationRefusesExistingCaptureFault(t *testing.T) {
	for name, action := range map[string]func(*Search){
		"delete row":  func(s *Search) { s.deleteInstance(1) },
		"replace row": func(s *Search) { s.replaceInstance(1) },
		"delete all":  (*Search).doDeleteAll,
		"replace all": (*Search).doReplaceAll,
	} {
		t.Run(name, func(t *testing.T) {
			s, a := searchOperationFixture(t)
			e := a.current
			e.Dmm().Tiles[0].Instances()[0].SetStableID("invalid-capture")
			e.BeginTileChange(e.Dmm().Tiles[0].Coord)
			if _, err := e.SaveSnapshot(context.Background()); err == nil {
				t.Fatal("fixture did not establish the capture fault")
			}
			s.Sync()
			before := e.Dmm().Copy()
			action(s)
			if !reflect.DeepEqual(e.Dmm().Copy(), before) || a.commands.HasUndoV(e.Dmm().Path.Absolute) {
				t.Error("search mutated a faulted map before rejecting the operation")
			}
		})
	}
}

func TestSearchCaptureFailureDoesNotPartiallyMutate(t *testing.T) {
	for name, action := range map[string]func(*Search){
		"delete row":  func(s *Search) { s.deleteInstance(2) },
		"replace row": func(s *Search) { s.replaceInstance(2) },
		"delete all":  (*Search).doDeleteAll,
		"replace all": (*Search).doReplaceAll,
	} {
		t.Run(name, func(t *testing.T) {
			s, a := searchOperationFixture(t)
			e := a.current
			// Damage the last result before its first capture, after earlier bulk
			// results would already have been mutated by a one-at-a-time loop.
			s.results()[2].SetStableID("invalid-later-capture")
			before := e.Dmm().Copy()
			action(s)
			if !reflect.DeepEqual(e.Dmm().Copy(), before) {
				t.Error("capture failure changed the row or an earlier bulk result")
			}
			if _, ready := e.MapViewVersion(); !ready {
				t.Error("failed search preflight retained unused tile captures")
			}
			if e.CanStartMapEdit() || a.commands.HasUndoV(e.Dmm().Path.Absolute) || len(a.errors) != 1 {
				t.Error("capture fault was not retained/reported without a history entry")
			}
		})
	}
}

func TestSearchReplacementPreflightLeavesMapUsable(t *testing.T) {
	for _, invalid := range []struct {
		name string
		make func() *dmmprefab.Prefab
	}{
		{"nil variables", func() *dmmprefab.Prefab {
			return dmmprefab.New(dmmprefab.IdStage, "/obj/replacement", nil)
		}},
		{"missing variable value", func() *dmmprefab.Prefab {
			vars := dmvars.Set(dmvars.FromParent(nil), "dir", "2")
			// Iterate exposes the names slice. A malformed selected prefab must
			// not be installed before CaptureTile notices the missing value.
			vars.Iterate()[0] = "missing"
			return dmmprefab.New(dmmprefab.IdStage, "/obj/replacement", vars)
		}},
		{"empty path", func() *dmmprefab.Prefab {
			return dmmprefab.New(dmmprefab.IdStage, "", dmvars.FromParent(nil))
		}},
	} {
		for name, action := range map[string]func(*Search){
			"row": func(s *Search) { s.replaceInstance(2) },
			"all": (*Search).doReplaceAll,
		} {
			t.Run(invalid.name+"/"+name, func(t *testing.T) {
				s, a := searchOperationFixture(t)
				e := a.current
				beforeSnapshot, beforeHash := searchAuthority(t, e)
				beforeDisplay := e.Dmm().Copy()
				a.selected = invalid.make()
				func() {
					defer func() {
						if value := recover(); value != nil {
							t.Errorf("invalid replacement panicked: %v", value)
						}
					}()
					action(s)
				}()
				if !reflect.DeepEqual(e.Dmm().Copy(), beforeDisplay) {
					t.Error("invalid replacement changed display contents")
				}
				if !e.CanStartMapEdit() || a.commands.HasUndoV(e.Dmm().Path.Absolute) || len(a.errors) != 1 {
					t.Fatalf("invalid replacement left an unusable map, history, or no single error: ready=%v errors=%v", e.CanStartMapEdit(), a.errors)
				}
				after, hash := searchAuthority(t, e)
				if hash != beforeHash || after.Revision != beforeSnapshot.Revision {
					t.Fatal("invalid replacement changed authority")
				}
				// The selected prefab was bad; the map itself remains healthy.
				a.selected = dmmap.PrefabStorage.Initial("/obj/replacement")
				action(s)
				after, _ = searchAuthority(t, e)
				if after.Revision != beforeSnapshot.Revision+1 || len(a.errors) != 1 {
					t.Fatal("valid retry did not publish exactly one operation")
				}
			})
		}
	}
}

func TestSearchReplacementPreservesUnknownPathAndVariables(t *testing.T) {
	s, a := searchOperationFixture(t)
	e := a.current
	_, before := searchAuthority(t, e)
	vars := dmvars.Set(dmvars.FromParent(nil), "opaque_value", "list(\"unknown\" = /obj/unavailable)")
	a.selected = dmmprefab.New(dmmprefab.IdNone, "/obj/unavailable", vars)
	s.doReplaceAll()
	after, _ := searchAuthority(t, e)
	replacements := 0
	for _, tile := range after.Tiles {
		for _, prefab := range tile.State.Prefabs {
			if prefab.Path == a.selected.Path() {
				replacements++
				if prefab.Vars["opaque_value"] != vars.ValueV("opaque_value", "") {
					t.Fatal("replacement changed an unknown variable value")
				}
			}
		}
	}
	if after.Revision != 1 || replacements != 3 || len(a.errors) != 0 {
		t.Fatal("capturable unknown content was rejected or partially replaced")
	}
	a.commands.UndoV(e.Dmm().Path.Absolute)
	_, undone := searchAuthority(t, e)
	if undone != before {
		t.Fatal("unknown replacement undo did not restore exact map hash")
	}
}
