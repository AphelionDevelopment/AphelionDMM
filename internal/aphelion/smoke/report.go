package smoke

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"sdmm/internal/dmapi/dmmap/dmmdata"
)

// StepResult records one smoke step without obscuring its diagnostic.
type StepResult struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Report records reproducible evidence from one local smoke run.
type Report struct {
	Revision      string               `json:"revision"`
	FixtureSHA256 string               `json:"fixture_sha256"`
	OutputSHA256  string               `json:"output_sha256,omitempty"`
	Open          StepResult           `json:"open"`
	Save          StepResult           `json:"save"`
	Shutdown      StepResult           `json:"shutdown"`
	Collaboration *CollaborationReport `json:"collaboration,omitempty"`
}

// OK reports whether every required smoke step completed successfully.
func (report Report) OK() bool {
	return report.Open.OK && report.Save.OK && report.Shutdown.OK && (report.Collaboration == nil || report.Collaboration.OK())
}

// Driver defines the trusted local operations exercised by a smoke run.
type Driver interface {
	Open(ctx context.Context, fixturePath string) error
	Save(ctx context.Context, outputPath string) error
	Shutdown(ctx context.Context) error
}

// ParserDriver exercises the inherited DMM parser and writer without UI automation.
type ParserDriver struct {
	data *dmmdata.DmmData
}

// NewParserDriver creates a local parser smoke driver.
func NewParserDriver() *ParserDriver {
	return &ParserDriver{}
}

// Open parses the trusted fixture through the inherited file entry point.
func (driver *ParserDriver) Open(ctx context.Context, fixturePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := dmmdata.New(fixturePath)
	if err != nil {
		return err
	}
	driver.data = data
	return nil
}

// Save writes and reopens the round-trip output.
func (driver *ParserDriver) Save(ctx context.Context, outputPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if driver.data == nil {
		return fmt.Errorf("save smoke output: no fixture is open")
	}
	if err := driver.data.SaveDM(outputPath); err != nil {
		return fmt.Errorf("save smoke output: %w", err)
	}
	if _, err := dmmdata.New(outputPath); err != nil {
		return fmt.Errorf("validate smoke output: %w", err)
	}
	return nil
}

// Shutdown releases the parser driver's loaded state.
func (driver *ParserDriver) Shutdown(ctx context.Context) error {
	driver.data = nil
	return ctx.Err()
}

// Run executes a local smoke round trip and records each result.
func Run(ctx context.Context, driver Driver, fixturePath, outputPath, revision string) Report {
	report := Report{Revision: revision}

	fixtureHash, err := hashFile(fixturePath)
	if err != nil {
		report.Open.Detail = fmt.Sprintf("hash fixture: %v", err)
	} else {
		report.FixtureSHA256 = fixtureHash
		if err := driver.Open(ctx, fixturePath); err != nil {
			report.Open.Detail = err.Error()
		} else {
			report.Open.OK = true
			if err := driver.Save(ctx, outputPath); err != nil {
				report.Save.Detail = err.Error()
			} else if outputHash, err := hashFile(outputPath); err != nil {
				report.Save.Detail = fmt.Sprintf("hash output: %v", err)
			} else {
				report.OutputSHA256 = outputHash
				report.Save.OK = true
			}
		}
	}

	if err := driver.Shutdown(ctx); err != nil {
		report.Shutdown.Detail = err.Error()
	} else {
		report.Shutdown.OK = true
	}
	return report
}

// Write serializes one smoke report as indented JSON.
func Write(writer io.Writer, report Report) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode smoke report: %w", err)
	}
	return nil
}

func hashFile(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(contents)
	return hex.EncodeToString(hash[:]), nil
}
