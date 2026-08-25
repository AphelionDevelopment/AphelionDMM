package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdmm/internal/aphelion/smoke"
)

func TestRunWritesSuccessfulSmokeReport(t *testing.T) {
	t.Parallel()

	fixturePath := filepath.Join("..", "..", "internal", "aphelion", "smoke", "testdata", "normal.dmm")
	outputDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(context.Background(), &stdout, &stderr, fixturePath, outputDir, "test-revision", smoke.NewParserDriver())
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=ok") {
		t.Fatalf("run() stdout = %q, want successful status", stdout.String())
	}

	reportFile, err := os.Open(filepath.Join(outputDir, reportFilename))
	if err != nil {
		t.Fatalf("open smoke report: %v", err)
	}
	defer func() { _ = reportFile.Close() }()

	var report smoke.Report
	if err := json.NewDecoder(reportFile).Decode(&report); err != nil {
		t.Fatalf("decode smoke report: %v", err)
	}
	if !report.OK() {
		t.Fatalf("run() report = %#v, want all steps successful", report)
	}
	if report.Collaboration == nil || !report.Collaboration.OK() {
		t.Fatalf("run() collaboration report = %#v, want successful two-client smoke", report.Collaboration)
	}
}
