package compat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

func TestV1WireFixtures(t *testing.T) {
	directory := filepath.Join("..", "..", "..", "..", "testdata", "collaboration", "compat", "v1")
	accepted, err := os.ReadFile(filepath.Join(directory, "accepted-optional-acknowledged-revision.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.DecodeClient(accepted); err != nil {
		t.Fatalf("accepted v1 fixture: %v", err)
	}
	rejected, err := os.ReadFile(filepath.Join(directory, "rejected-changed-semantics.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.DecodeClient(rejected); err == nil {
		t.Fatal("changed-semantics fixture was accepted")
	}
}

func TestV1SnapshotFixtureHash(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "testdata", "collaboration", "compat", "v1", "snapshot.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Snapshot        model.Snapshot `json:"snapshot"`
		ExpectedMapHash string         `json:"expected_map_hash"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	hash, err := fixture.Snapshot.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if hash != fixture.ExpectedMapHash {
		t.Fatalf("snapshot hash = %q, want %q", hash, fixture.ExpectedMapHash)
	}
}
