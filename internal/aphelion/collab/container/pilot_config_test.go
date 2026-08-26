package container_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPilotPostgresHealthCheckUsesTCPListener(t *testing.T) {
	t.Parallel()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate pilot configuration test source")
	}
	composePath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "..", "deploy", "pilot", "compose.yaml")
	contents, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	want := `test: ["CMD-SHELL", "pg_isready -h 127.0.0.1 -U aphelion_pilot -d aphelion_pilot"]`
	if !strings.Contains(string(contents), want) {
		t.Fatalf("pilot PostgreSQL health check does not probe the TCP listener")
	}
}
