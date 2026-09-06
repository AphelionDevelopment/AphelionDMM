package load

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/engine"
)

func TestConcurrentScenarioHasOrderIndependentOutcomes(t *testing.T) {
	config := ConcurrentConfig{Config: Config{Seed: 260906, Clients: 4, Operations: 20, PresencePerClient: 4, MaxX: 5, MaxY: 4}, ConflictPairs: 5}
	scenario, err := GenerateConcurrent(config)
	if err != nil {
		t.Fatal(err)
	}
	again, err := GenerateConcurrent(config)
	if err != nil || !reflect.DeepEqual(scenario, again) {
		t.Fatal("generation is not deterministic")
	}
	initialHash, err := scenario.Initial.Hash()
	if err != nil {
		t.Fatal(err)
	}
	for _, intent := range scenario.Intents {
		if intent.Operation.BaseRevision != 0 || intent.Operation.BaseMapHash != initialHash {
			t.Fatal("concurrent intent depends on an earlier acceptance")
		}
	}
	random := rand.New(rand.NewSource(260906))
	for trial := 0; trial < 20; trial++ {
		document, err := engine.NewDocument(scenario.Initial)
		if err != nil {
			t.Fatal(err)
		}
		accepted, rejected := 0, 0
		for _, index := range random.Perm(len(scenario.Intents)) {
			if _, err := document.Apply(scenario.Intents[index].Operation, time.Unix(0, 1)); err == nil {
				accepted++
			} else {
				rejected++
			}
		}
		hash, err := document.Snapshot().Hash()
		if err != nil || hash != scenario.ExpectedMapHash || accepted != 15 || rejected != 5 {
			t.Fatalf("order changed outcome: %d accepted, %d rejected, hash=%s err=%v", accepted, rejected, hash, err)
		}
	}
	if err := scenario.Verify(); err != nil {
		t.Fatal(err)
	}
	scenario.Intents[0].Operation.Changes[0].After.Prefabs[0].Path = "/altered"
	if err := scenario.Verify(); err == nil {
		t.Fatal("altered scenario verified")
	}
}

func TestConcurrentScenarioRejectsInvalidShapes(t *testing.T) {
	for _, config := range []ConcurrentConfig{
		{},
		{Config: Config{Seed: 1, Clients: 1, Operations: 2, MaxX: 2, MaxY: 1}, ConflictPairs: 1},
		{Config: Config{Seed: 1, Clients: 2, Operations: 2, MaxX: 2, MaxY: 1}, ConflictPairs: 2},
		{Config: Config{Seed: 1, Clients: 2, Operations: 3, MaxX: 2, MaxY: 1}},
		{Config: Config{Seed: 1, Clients: 2, Operations: 2, MaxX: 2, MaxY: 1}, ConflictPairs: -1},
	} {
		if _, err := GenerateConcurrent(config); err == nil {
			t.Errorf("accepted invalid shape %#v", config)
		}
	}
}
