package load

import (
	"reflect"
	"testing"
)

func TestGenerateScenarioIsDeterministicAndSelfVerifying(t *testing.T) {
	config := Config{Seed: 20260825, Clients: 25, Operations: 250, PresencePerClient: 20, MaxX: 10, MaxY: 10}
	first, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same seed generated different scenarios")
	}
	if len(first.Actors) != config.Clients || len(first.Operations) != config.Operations || len(first.Presence) != config.Clients*config.PresencePerClient {
		t.Fatalf("scenario counts = actors %d operations %d presence %d", len(first.Actors), len(first.Operations), len(first.Presence))
	}
	if first.ExpectedRevision != 250 || len(first.ExpectedMapHash) != 64 {
		t.Fatalf("expected result = revision %d hash %q", first.ExpectedRevision, first.ExpectedMapHash)
	}
	if err := first.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateScenarioRejectsUnboundedInputs(t *testing.T) {
	for _, config := range []Config{
		{},
		{Seed: 1, Clients: 101, Operations: 1, MaxX: 1, MaxY: 1},
		{Seed: 1, Clients: 1, Operations: 100001, MaxX: 1, MaxY: 1},
		{Seed: 1, Clients: 1, Operations: 1, PresencePerClient: 1001, MaxX: 1, MaxY: 1},
	} {
		if _, err := Generate(config); err == nil {
			t.Errorf("Generate(%#v) error = nil", config)
		}
	}
}

func TestScenarioVerifyRejectsAlteredBoundedCounts(t *testing.T) {
	scenario, err := Generate(Config{Seed: 7, Clients: 2, Operations: 2, PresencePerClient: 1, MaxX: 2, MaxY: 1})
	if err != nil {
		t.Fatal(err)
	}
	scenario.Actors = append(scenario.Actors, scenario.Actors[0])
	if err := scenario.Verify(); err == nil {
		t.Fatal("Verify() error = nil for altered actor count")
	}
}
