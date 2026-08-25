package buildcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Manifest records the toolchain versions required by this repository.
type Manifest struct {
	Go           string `json:"go"`
	Rust         string `json:"rust"`
	RustTarget   string `json:"rust_target"`
	TaskMajor    int    `json:"task_major"`
	GolangCILint string `json:"golangci_lint"`
}

// DecodeManifest decodes and validates one strict toolchain manifest.
func DecodeManifest(reader io.Reader) (Manifest, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode toolchain manifest: %w", err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Manifest{}, fmt.Errorf("decode toolchain manifest: trailing JSON value")
		}
		return Manifest{}, fmt.Errorf("decode toolchain manifest: trailing JSON: %w", err)
	}

	if err := manifest.validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (manifest Manifest) validate() error {
	for name, value := range map[string]string{
		"go":            manifest.Go,
		"rust":          manifest.Rust,
		"rust_target":   manifest.RustTarget,
		"golangci_lint": manifest.GolangCILint,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("toolchain manifest: %s is required", name)
		}
	}
	if manifest.TaskMajor <= 0 {
		return fmt.Errorf("toolchain manifest: task_major must be positive")
	}
	return nil
}
