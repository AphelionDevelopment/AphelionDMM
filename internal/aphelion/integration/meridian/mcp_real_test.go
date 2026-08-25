package meridian

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMCPRealInstalledContract(t *testing.T) {
	executable := os.Getenv("APHELION_MERIDIAN_MCP_REAL")
	root := os.Getenv("APHELION_MERIDIAN_RIFT_ROOT")
	if executable == "" || root == "" {
		t.Skip("set APHELION_MERIDIAN_MCP_REAL and APHELION_MERIDIAN_RIFT_ROOT for the installed contract gate")
	}

	client, err := NewMCP(context.Background(), Config{
		Executable: executable,
		Environment: map[string]string{
			"MERIDIAN_MCP_MODE":  "analysis",
			"MERIDIAN_MCP_ROOTS": root,
		},
		Roots: map[string]Repository{
			"meridian-rift": {
				Identity: "meridian-rift",
				Root:     root,
				DME:      "tgstation.dme",
				Targets: map[string]string{
					"test-map": filepath.ToSlash(filepath.Join("_maps", "virtual_domains", "test_only.dmm")),
				},
			},
		},
		Timeout:  5 * time.Minute,
		MaxBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("NewMCP() error = %v", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Close(closeCtx); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}()

	parsed, err := client.ParseEnvironment(context.Background(), "meridian-rift", "tgstation.dme")
	if err != nil {
		t.Fatalf("ParseEnvironment() error = %v", err)
	}
	if parsed.StateGeneration == 0 || parsed.MCPVersion == "" {
		t.Fatalf("ParseEnvironment() returned incomplete metadata: %#v", parsed)
	}
	if _, err := client.InspectMap(context.Background(), "meridian-rift", "test-map"); err != nil {
		t.Fatalf("InspectMap() error = %v", err)
	}
	if _, err := client.CheckErrors(context.Background(), "meridian-rift"); err != nil {
		t.Fatalf("CheckErrors() error = %v", err)
	}
}
