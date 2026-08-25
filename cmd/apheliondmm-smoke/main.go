package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/smoke"
	"sdmm/internal/env"
)

const (
	fixturePath    = "internal/aphelion/smoke/testdata/normal.dmm"
	reportFilename = "report.json"
	outputFilename = "roundtrip.dmm"
)

func main() {
	outputDir, err := os.MkdirTemp("", "apheliondmm-smoke-")
	if err != nil {
		reportError(os.Stderr, "create smoke output directory: %v", err)
		os.Exit(1)
	}
	os.Exit(run(context.Background(), os.Stdout, os.Stderr, fixturePath, outputDir, env.Revision, smoke.NewParserDriver()))
}

func run(ctx context.Context, stdout, stderr io.Writer, fixture, outputDir, revision string, driver smoke.Driver) int {
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		reportError(stderr, "create smoke output directory: %v", err)
		return 1
	}

	report := smoke.Run(ctx, driver, fixture, filepath.Join(outputDir, outputFilename), revision)
	documentID, err := model.NewDocumentID()
	if err != nil {
		report.Collaboration = &smoke.CollaborationReport{ServiceLaunch: smoke.StepResult{Detail: fmt.Sprintf("create smoke document identity: %v", err)}}
	} else {
		collaboration := smoke.RunCollaboration(ctx, model.Snapshot{
			ProtocolVersion: model.ProtocolVersion,
			SchemaVersion:   model.SchemaVersion,
			DocumentID:      documentID,
			EnvironmentHash: strings.Repeat("a", 64),
			MaxX:            2,
			MaxY:            1,
			MaxZ:            1,
		})
		report.Collaboration = &collaboration
	}
	reportPath := filepath.Join(outputDir, reportFilename)
	reportFile, err := os.Create(reportPath)
	if err != nil {
		reportError(stderr, "create smoke report: %v", err)
		return 1
	}
	writeErr := smoke.Write(reportFile, report)
	closeErr := reportFile.Close()
	if writeErr != nil {
		reportError(stderr, "%v", writeErr)
		return 1
	}
	if closeErr != nil {
		reportError(stderr, "close smoke report: %v", closeErr)
		return 1
	}

	status := "ok"
	exitCode := 0
	if !report.OK() {
		status = "fail"
		exitCode = 1
	}
	if _, err := fmt.Fprintf(stdout, "status=%s report=%q\n", status, reportPath); err != nil {
		reportError(stderr, "write smoke result: %v", err)
		return 1
	}
	return exitCode
}

func reportError(writer io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(writer, format+"\n", args...)
}
