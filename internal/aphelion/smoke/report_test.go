package smoke

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const fixtureSHA256 = "7d044f7165ca68f32d823f463c6810876c91aa13934d4880fd96810469e9c9be"

func TestRunRecordsParserRoundTrip(t *testing.T) {
	t.Parallel()

	fixturePath := filepath.Join("testdata", "normal.dmm")
	outputPath := filepath.Join(t.TempDir(), "roundtrip.dmm")
	report := Run(context.Background(), NewParserDriver(), fixturePath, outputPath, "test-revision")

	if report.Revision != "test-revision" {
		t.Fatalf("Run() revision = %q, want test-revision", report.Revision)
	}
	if report.FixtureSHA256 != fixtureSHA256 {
		t.Fatalf("Run() fixture hash = %q, want %q", report.FixtureSHA256, fixtureSHA256)
	}
	for name, step := range map[string]StepResult{
		"open":     report.Open,
		"save":     report.Save,
		"shutdown": report.Shutdown,
	} {
		if !step.OK {
			t.Errorf("Run() %s step failed: %s", name, step.Detail)
		}
	}

	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read round-trip output: %v", err)
	}
	wantOutputHash := sha256.Sum256(contents)
	if report.OutputSHA256 != hex.EncodeToString(wantOutputHash[:]) {
		t.Fatalf("Run() output hash = %q, want %x", report.OutputSHA256, wantOutputHash)
	}
}

func TestWriteProducesJSONReport(t *testing.T) {
	t.Parallel()

	want := Report{
		Revision:      "test-revision",
		FixtureSHA256: fixtureSHA256,
		OutputSHA256:  "output-hash",
		Open:          StepResult{OK: true},
		Save:          StepResult{OK: true},
		Shutdown:      StepResult{OK: true},
	}
	var output bytes.Buffer
	if err := Write(&output, want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	var got Report
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if got != want {
		t.Fatalf("Write() report = %#v, want %#v", got, want)
	}
}
