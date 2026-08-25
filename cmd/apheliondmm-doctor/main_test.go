package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type doctorRunner struct {
	failingCommand string
}

type errorWriter struct{}

func (errorWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

func (runner doctorRunner) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	if name == runner.failingCommand {
		return nil, errors.New("not found")
	}
	outputs := map[string]string{
		"go":            "go version go1.25.13 windows/amd64",
		"rustup":        "rustc 1.82.0 (test)",
		"task":          "3.53.1",
		"golangci-lint": "golangci-lint has version v2.12.2 built with go1.26.2",
		"gcc":           "gcc.exe (GCC) 15.2.0",
	}
	return []byte(outputs[name]), nil
}

func TestRunReportsCompatibleEnvironment(t *testing.T) {
	t.Parallel()

	manifestPath := writeManifest(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(context.Background(), &stdout, &stderr, manifestPath, doctorRunner{})
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q, want empty", stderr.String())
	}
	for _, tool := range []string{"go", "rust", "task", "golangci-lint", "gcc"} {
		if !strings.Contains(stdout.String(), "status=ok tool="+tool+" ") {
			t.Errorf("run() stdout missing successful %s result:\n%s", tool, stdout.String())
		}
	}
}

func TestRunReturnsFailureWhenRequiredToolIsMissing(t *testing.T) {
	t.Parallel()

	manifestPath := writeManifest(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(context.Background(), &stdout, &stderr, manifestPath, doctorRunner{failingCommand: "gcc"})
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout.String(), `status=fail tool=gcc expected="available" actual="" detail="command failed: not found"`) {
		t.Fatalf("run() stdout missing failed gcc result:\n%s", stdout.String())
	}
}

func TestRunReturnsFailureWhenReportCannotBeWritten(t *testing.T) {
	t.Parallel()

	manifestPath := writeManifest(t)
	var stderr bytes.Buffer

	exitCode := run(context.Background(), errorWriter{}, &stderr, manifestPath, doctorRunner{})
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "write environment report: write failed") {
		t.Fatalf("run() stderr = %q, want report-write error", stderr.String())
	}
}

func writeManifest(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "manifest.json")
	contents := `{
		"go": "1.25.13",
		"rust": "1.82.0",
		"rust_target": "x86_64-pc-windows-gnu",
		"task_major": 3,
		"golangci_lint": "2.12.2"
	}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}
