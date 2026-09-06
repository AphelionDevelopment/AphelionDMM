package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	loadscenario "sdmm/internal/aphelion/collab/load"
)

func loadConcurrentScenario(path string) (loadscenario.ConcurrentScenario, error) {
	file, err := os.Open(path)
	if err != nil {
		return loadscenario.ConcurrentScenario{}, err
	}
	defer func() { _ = file.Close() }()
	const maximumBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil {
		return loadscenario.ConcurrentScenario{}, err
	}
	if len(data) > maximumBytes {
		return loadscenario.ConcurrentScenario{}, fmt.Errorf("concurrent manifest exceeds %d bytes", maximumBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var config loadscenario.ConcurrentConfig
	if err := decoder.Decode(&config); err != nil {
		return loadscenario.ConcurrentScenario{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return loadscenario.ConcurrentScenario{}, fmt.Errorf("concurrent manifest must contain exactly one JSON object")
	}
	return loadscenario.GenerateConcurrent(config)
}

func runConcurrentCommand(ctx context.Context, config loadscenario.RunConfig, path string, output, errorOutput io.Writer) int {
	scenario, err := loadConcurrentScenario(path)
	if err != nil {
		_, _ = fmt.Fprintf(errorOutput, "load concurrent scenario: %v\n", err)
		return 1
	}
	result, runErr := loadscenario.RunConcurrent(ctx, config, scenario)
	// Failed runs retain their partial counts and failure reason for comparison.
	if err := json.NewEncoder(output).Encode(result); err != nil {
		_, _ = fmt.Fprintf(errorOutput, "write concurrent load result: %v\n", err)
		return 1
	}
	if runErr != nil {
		_, _ = fmt.Fprintf(errorOutput, "run concurrent load scenario: %v\n", runErr)
		return 1
	}
	if !result.GatePassed {
		return 1
	}
	return 0
}
