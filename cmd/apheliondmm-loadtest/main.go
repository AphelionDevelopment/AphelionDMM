package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	loadscenario "sdmm/internal/aphelion/collab/load"
)

const editorTokensFileEnvironment = "APHELIONDMM_LOAD_EDITOR_TOKENS_FILE"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, output, errorOutput io.Writer) int {
	flags := flag.NewFlagSet("apheliondmm-loadtest", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	endpoint := flags.String("endpoint", "", "collaboration service HTTPS endpoint")
	origin := flags.String("origin", "", "configured WebSocket Origin")
	sessionID := flags.String("session", "", "existing collaboration session ID")
	scenarioPath := flags.String("scenario", "", "recorded scenario JSON (defaults to pilot.json, or concurrent.json with -concurrent, under testdata/collaboration/load)")
	concurrent := flags.Bool("concurrent", false, "offer independent intents and presence concurrently; verify every client's applied state")
	timeout := flags.Duration("timeout", 10*time.Minute, "overall scenario timeout")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	ownerToken := os.Getenv("APHELIONDMM_LOAD_OWNER_TOKEN")
	if ownerToken == "" {
		_, _ = fmt.Fprintln(errorOutput, "APHELIONDMM_LOAD_OWNER_TOKEN is required")
		return 2
	}
	editorTokens, err := editorTokensFromEnvironment()
	if err != nil {
		_, _ = fmt.Fprintf(errorOutput, "load hosted editor credentials: %v\n", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	config := loadscenario.RunConfig{Endpoint: *endpoint, Origin: *origin, SessionID: *sessionID, OwnerToken: ownerToken, EditorTokens: editorTokens}
	if *concurrent {
		if *scenarioPath == "" {
			*scenarioPath = "testdata/collaboration/load/concurrent.json"
		}
		return runConcurrentCommand(ctx, config, *scenarioPath, output, errorOutput)
	}
	if *scenarioPath == "" {
		*scenarioPath = "testdata/collaboration/load/pilot.json"
	}
	scenario, err := loadScenario(*scenarioPath)
	if err != nil {
		_, _ = fmt.Fprintf(errorOutput, "load scenario: %v\n", err)
		return 1
	}
	result, err := loadscenario.Run(ctx, config, scenario)
	if err != nil {
		_, _ = fmt.Fprintf(errorOutput, "run load scenario: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(output).Encode(result); err != nil {
		_, _ = fmt.Fprintf(errorOutput, "write load result: %v\n", err)
		return 1
	}
	if !result.GatePassed {
		return 1
	}
	return 0
}

func editorTokensFromEnvironment() ([]string, error) {
	path := os.Getenv(editorTokensFileEnvironment)
	if path == "" {
		return nil, nil
	}
	return loadscenario.LoadEditorTokens(path)
}

func loadScenario(path string) (loadscenario.Scenario, error) {
	file, err := os.Open(path)
	if err != nil {
		return loadscenario.Scenario{}, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(io.LimitReader(file, (1<<20)+1))
	decoder.DisallowUnknownFields()
	var recorded loadscenario.Config
	if err := decoder.Decode(&recorded); err != nil {
		return loadscenario.Scenario{}, err
	}
	if recorded.TargetOperationsPerSecond <= 0 || recorded.TargetPresencePerSecondPerEditor <= 0 || recorded.MaximumP95AcknowledgementMilliseconds <= 0 {
		return loadscenario.Scenario{}, fmt.Errorf("recorded pilot targets must be positive")
	}
	return loadscenario.Generate(recorded)
}
