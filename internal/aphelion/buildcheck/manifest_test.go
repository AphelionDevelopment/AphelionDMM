package buildcheck

import (
	"strings"
	"testing"
)

func TestDecodeManifest(t *testing.T) {
	t.Parallel()

	manifest, err := DecodeManifest(strings.NewReader(`{
		"go": "1.24.0",
		"rust": "1.82.0",
		"rust_target": "x86_64-pc-windows-gnu",
		"task_major": 3,
		"golangci_lint": "2.1.5"
	}`))
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}

	want := Manifest{
		Go:           "1.24.0",
		Rust:         "1.82.0",
		RustTarget:   "x86_64-pc-windows-gnu",
		TaskMajor:    3,
		GolangCILint: "2.1.5",
	}
	if manifest != want {
		t.Fatalf("DecodeManifest() = %#v, want %#v", manifest, want)
	}
}

func TestDecodeManifestRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := DecodeManifest(strings.NewReader(`{
		"go": "1.24.0",
		"rust": "1.82.0",
		"rust_target": "x86_64-pc-windows-gnu",
		"task_major": 3,
		"golangci_lint": "2.1.5",
		"unexpected": true
	}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("DecodeManifest() error = %v, want unknown-field error", err)
	}
}

func TestDecodeManifestRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	_, err := DecodeManifest(strings.NewReader(`{
		"go": "1.24.0",
		"rust": "1.82.0",
		"rust_target": "x86_64-pc-windows-gnu",
		"task_major": 3,
		"golangci_lint": "2.1.5"
	} {}`))
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("DecodeManifest() error = %v, want trailing-data error", err)
	}
}

func TestDecodeManifestRejectsMissingRequiredValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		manifest string
		missing  string
	}{
		{name: "go", manifest: `{"rust":"1.82.0","rust_target":"x86_64-pc-windows-gnu","task_major":3,"golangci_lint":"2.1.5"}`, missing: "go"},
		{name: "rust", manifest: `{"go":"1.24.0","rust_target":"x86_64-pc-windows-gnu","task_major":3,"golangci_lint":"2.1.5"}`, missing: "rust"},
		{name: "rust target", manifest: `{"go":"1.24.0","rust":"1.82.0","task_major":3,"golangci_lint":"2.1.5"}`, missing: "rust_target"},
		{name: "task major", manifest: `{"go":"1.24.0","rust":"1.82.0","rust_target":"x86_64-pc-windows-gnu","golangci_lint":"2.1.5"}`, missing: "task_major"},
		{name: "golangci-lint", manifest: `{"go":"1.24.0","rust":"1.82.0","rust_target":"x86_64-pc-windows-gnu","task_major":3}`, missing: "golangci_lint"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeManifest(strings.NewReader(test.manifest))
			if err == nil || !strings.Contains(err.Error(), test.missing) {
				t.Fatalf("DecodeManifest() error = %v, want error containing %q", err, test.missing)
			}
		})
	}
}
