package mapadapter

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

const (
	testDocumentID      = model.DocumentID("01890f3e-7b5c-7abc-8def-0123456789ab")
	testActorID         = model.ActorID("01890f3e-7b5c-7abc-8def-0123456789ba")
	testOperationID     = model.OperationID("01890f3e-7b5c-7abc-8def-0123456789bb")
	testEnvironmentHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestApplyAndExportRejectUnsafeDimensionsWithoutPanic(t *testing.T) {
	t.Parallel()

	snapshot := model.Snapshot{
		ProtocolVersion: model.ProtocolVersion,
		SchemaVersion:   model.SchemaVersion,
		DocumentID:      testDocumentID,
		EnvironmentHash: testEnvironmentHash,
		MaxX:            model.MaxMapDimension + 1,
		MaxY:            1,
		MaxZ:            1,
	}
	target := &dmmap.Dmm{Name: "unchanged.dmm", MaxX: 1, MaxY: 1, MaxZ: 1}
	if err := Apply(target, snapshot); err == nil {
		t.Fatal("Apply() error = nil for unsafe dimensions")
	}
	if target.MaxX != 1 || target.MaxY != 1 || target.MaxZ != 1 || len(target.Tiles) != 0 {
		t.Fatalf("Apply() mutated target after rejection: %#v", target)
	}
	if _, err := Export(snapshot, "unsafe.dmm", false, "\n"); err == nil {
		t.Fatal("Export() error = nil for unsafe dimensions")
	}
}

func TestImportRejectsUnsafeDimensionsBeforeTileCount(t *testing.T) {
	t.Parallel()

	source := &dmmap.Dmm{MaxX: model.MaxMapDimension + 1, MaxY: 1, MaxZ: 1}
	_, err := Import(source, testDocumentID, testEnvironmentHash)
	if err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("Import() error = %v, want dimension maximum error", err)
	}
}

func TestUnknownContentRoundTrip(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)

	input, err := dmmdata.New(filepath.Join("testdata", "unknown-types.dmm"))
	if err != nil {
		t.Fatalf("parse input fixture: %v", err)
	}
	source, unknownPrefabs := dmmap.New(testEnvironment(input.Filepath), input, input.Filepath)
	if _, exists := unknownPrefabs["/obj/unknown_aphelion"]; !exists {
		t.Fatal("dmmap.New() did not report the unknown prefab")
	}
	if got := len(source.Tiles[1].Instances()); got != 3 {
		t.Fatalf("dmmap.New() retained %d prefabs on unknown tile, want 3", got)
	}
	snapshot, err := Import(source, testDocumentID, testEnvironmentHash)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if snapshot.MaxZ != 2 {
		t.Fatalf("snapshot MaxZ = %d, want 2", snapshot.MaxZ)
	}
	unknown := snapshot.Tiles[1].State.Prefabs[2]
	if unknown.Path != "/obj/unknown_aphelion" || unknown.Vars["opaque"] != "list(1, 2)" {
		t.Fatalf("unknown prefab was not preserved: %#v", unknown)
	}
	if err := unknown.StableID.Validate(); err != nil {
		t.Fatalf("unknown prefab stable ID is invalid: %v", err)
	}
	if got := snapshot.Tiles[0].State.Prefabs[2].Vars["text"]; got != `"a\"b"` {
		t.Fatalf("escaped variable = %q, want %q", got, `"a\"b"`)
	}

	document, err := engine.NewDocument(snapshot)
	if err != nil {
		t.Fatalf("engine.NewDocument() error = %v", err)
	}
	before := model.CloneTileState(snapshot.Tiles[1].State)
	after := model.CloneTileState(before)
	after.Prefabs[2].Vars["opaque"] = "list(1, 2, 3)"
	baseHash, err := snapshot.Hash()
	if err != nil {
		t.Fatalf("hash imported snapshot: %v", err)
	}
	operation := model.Operation{
		ProtocolVersion: model.ProtocolVersion,
		DocumentID:      snapshot.DocumentID,
		ActorID:         testActorID,
		OperationID:     testOperationID,
		BaseRevision:    snapshot.Revision,
		EnvironmentHash: snapshot.EnvironmentHash,
		BaseMapHash:     baseHash,
		Kind:            model.OperationKindTileChange,
		Changes: []model.TileChange{{
			Coord:  snapshot.Tiles[1].Coord,
			Before: before,
			After:  after,
		}},
	}
	if _, err := document.Apply(operation, time.Unix(1, 0)); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	authoritative := document.Snapshot()

	dmmap.PrefabStorage.Free()
	target := &dmmap.Dmm{Name: "roundtrip.dmm"}
	if err := ApplyWithEnvironment(target, authoritative, testEnvironment(input.Filepath)); err != nil {
		t.Fatalf("Apply(map) error = %v", err)
	}
	if got := target.Tiles[0].Instances()[2].Prefab().Vars().ValueV("parent_only", ""); got != `"value"` {
		t.Fatalf("applied known prefab inherited variable = %q, want %q", got, `"value"`)
	}
	applied, err := Reimport(target, authoritative)
	if err != nil {
		t.Fatalf("Reimport(applied map) error = %v", err)
	}
	assertSameHash(t, applied, authoritative)

	outputPath := filepath.Join(t.TempDir(), "roundtrip.dmm")
	output, err := Export(authoritative, outputPath, false, "\n")
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if err := output.SaveDM(outputPath); err != nil {
		t.Fatalf("save exported fixture: %v", err)
	}
	reparsed, err := dmmdata.New(outputPath)
	if err != nil {
		t.Fatalf("parse exported fixture: %v", err)
	}
	roundtrip, err := Reimport(dmmFromData(reparsed), authoritative)
	if err != nil {
		t.Fatalf("Reimport(reparsed map) error = %v", err)
	}
	assertSameHash(t, roundtrip, authoritative)
	if got := roundtrip.Tiles[1].State.Prefabs[2].Vars["opaque"]; got != "list(1, 2, 3)" {
		t.Fatalf("round-trip opaque variable = %q, want %q", got, "list(1, 2, 3)")
	}
}

func testEnvironment(rootFile string) *dmenv.Dme {
	objects := make(map[string]*dmenv.Object)
	for _, path := range []string{"/area/foo", "/turf/foo", "/obj/foo1"} {
		mutable := &dmvars.MutableVariables{}
		if path == "/obj/foo1" {
			mutable.Put("parent_only", `"value"`)
		}
		variables := mutable.ToImmutable()
		objects[path] = &dmenv.Object{Path: path, Vars: variables}
	}
	return &dmenv.Dme{
		RootDir:  filepath.Dir(rootFile),
		RootFile: rootFile,
		Objects:  objects,
	}
}

func assertSameHash(t *testing.T, got model.Snapshot, want model.Snapshot) {
	t.Helper()
	gotHash, err := got.Hash()
	if err != nil {
		t.Fatalf("hash got snapshot: %v", err)
	}
	wantHash, err := want.Hash()
	if err != nil {
		t.Fatalf("hash wanted snapshot: %v", err)
	}
	if gotHash != wantHash {
		t.Fatalf("snapshot hash = %q, want %q", gotHash, wantHash)
	}
}

func dmmFromData(data *dmmdata.DmmData) *dmmap.Dmm {
	result := &dmmap.Dmm{
		Name:  data.Filepath,
		MaxX:  data.MaxX,
		MaxY:  data.MaxY,
		MaxZ:  data.MaxZ,
		Tiles: make([]*dmmap.Tile, 0, data.MaxX*data.MaxY*data.MaxZ),
	}
	for z := 1; z <= data.MaxZ; z++ {
		for y := 1; y <= data.MaxY; y++ {
			for x := 1; x <= data.MaxX; x++ {
				coord := util.Point{X: x, Y: y, Z: z}
				tile := &dmmap.Tile{Coord: coord}
				tile.InstancesSet(data.Dictionary[data.Grid[coord]])
				result.Tiles = append(result.Tiles, tile)
			}
		}
	}
	return result
}
