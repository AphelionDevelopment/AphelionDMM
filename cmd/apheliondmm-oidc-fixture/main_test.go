package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureTLSConfigSupportsTLS12AndNewer(t *testing.T) {
	config := fixtureTLSConfig(tls.Certificate{})
	if config.MinVersion != tls.VersionTLS12 {
		t.Fatalf("minimum TLS version = %x, want TLS 1.2", config.MinVersion)
	}
}

func TestRunRequiresExplicitFixtureConfiguration(t *testing.T) {
	var output bytes.Buffer
	if status := run(context.Background(), nil, &output, &output, func(string) (string, bool) { return "", false }); status != 2 {
		t.Fatalf("run status = %d output=%s", status, output.String())
	}
}

func TestWriteCAFileCreatesProtectedFileWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := writeCAFile(path, []byte("certificate")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "certificate" {
		t.Fatalf("CA file = %q error=%v", data, err)
	}
	if err := writeCAFile(path, []byte("replacement")); err == nil {
		t.Fatal("writeCAFile() overwrote an existing file")
	}
}
