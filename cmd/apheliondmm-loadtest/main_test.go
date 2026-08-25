package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestLoadScenarioReadsRecordedPilotSeed(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "collaboration", "load", "pilot.json")
	scenario, err := loadScenario(path)
	if err != nil {
		t.Fatal(err)
	}
	if scenario.Config.Seed != 20260825 || scenario.Config.Clients != 25 || scenario.ExpectedRevision != 250 {
		t.Fatalf("scenario = %#v", scenario)
	}
}

func TestRunRequiresOwnerTokenFromEnvironment(t *testing.T) {
	t.Setenv("APHELIONDMM_LOAD_OWNER_TOKEN", "")
	var output bytes.Buffer
	if code := run([]string{"-endpoint", "https://maps.example", "-origin", "https://maps.example", "-session", "session"}, &output, &output); code == 0 {
		t.Fatalf("run() = 0, output = %s", output.String())
	}
}
