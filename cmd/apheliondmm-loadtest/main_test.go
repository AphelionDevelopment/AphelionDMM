package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestRelayBundleModeDoesNotRequireHostedOwnerToken(t *testing.T) {
	t.Setenv("APHELIONDMM_LOAD_OWNER_TOKEN", "")
	var output bytes.Buffer
	code := run([]string{"-relay-bundle", filepath.Join(t.TempDir(), "missing.json")}, &output, &output)
	if code == 0 || strings.Contains(output.String(), "APHELIONDMM_LOAD_OWNER_TOKEN") {
		t.Fatalf("run() = %d, output = %s", code, output.String())
	}
}

func TestEditorTokensFromEnvironmentLoadsHostedCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "editor-tokens.json")
	if err := os.WriteFile(path, []byte(`{"tokens":["first","second"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APHELIONDMM_LOAD_EDITOR_TOKENS_FILE", path)
	tokens, err := editorTokensFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 || tokens[0] != "first" || tokens[1] != "second" {
		t.Fatalf("tokens = %#v", tokens)
	}
}

func TestRunRequiresOwnerTokenFromEnvironment(t *testing.T) {
	t.Setenv("APHELIONDMM_LOAD_OWNER_TOKEN", "")
	var output bytes.Buffer
	if code := run([]string{"-endpoint", "https://maps.example", "-origin", "https://maps.example", "-session", "session"}, &output, &output); code == 0 {
		t.Fatalf("run() = 0, output = %s", output.String())
	}
}
