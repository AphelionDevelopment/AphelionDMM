package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadSnapshotAndProtectedTokenFile(t *testing.T) {
	t.Parallel()

	snapshot, err := loadSnapshot(filepath.Join("..", "..", "testdata", "collaboration", "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.MaxX != 3 || snapshot.MaxY != 1 || snapshot.MaxZ != 1 {
		t.Fatalf("snapshot dimensions = %dx%dx%d", snapshot.MaxX, snapshot.MaxY, snapshot.MaxZ)
	}
	path := filepath.Join(t.TempDir(), "launch.token")
	if err := writeLaunchToken(path, "secret"); err != nil {
		t.Fatal(err)
	}
	if err := writeLaunchToken(path, "replacement"); err == nil {
		t.Fatal("writeLaunchToken replaced an existing token file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "secret" {
		t.Fatalf("token contents = %q", data)
	}
}

func TestRunExercisesServiceEntryPoint(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdoutReader, stdoutWriter := io.Pipe()
	defer func() { _ = stdoutReader.Close() }()
	var stderr bytes.Buffer
	tokenPath := filepath.Join(t.TempDir(), "launch.token")
	exitCodes := make(chan int, 1)
	go func() {
		exitCodes <- run(ctx, []string{
			"-snapshot-config", filepath.Join("..", "..", "testdata", "collaboration", "snapshot.json"),
			"-listen", "127.0.0.1:0",
			"-launch-token-file", tokenPath,
		}, stdoutWriter, &stderr)
		_ = stdoutWriter.Close()
	}()

	scanner := bufio.NewScanner(stdoutReader)
	if !scanner.Scan() {
		t.Fatalf("entry point produced no address: %s", stderr.String())
	}
	baseURL := strings.TrimSuffix(strings.TrimPrefix(scanner.Text(), "base_url=\""), "\"")
	client := &http.Client{Timeout: time.Second}
	for _, path := range []string{"/v1/health/live", "/v1/version"} {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			t.Fatalf("GET %s status = %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := loadSnapshot(filepath.Join("..", "..", "testdata", "collaboration", "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/v1/sessions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(token))
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d, want 201", response.StatusCode)
	}

	cancel()
	select {
	case exitCode := <-exitCodes:
		if exitCode != 0 {
			t.Fatalf("run() exit code = %d, stderr = %s", exitCode, stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("entry point did not shut down within two seconds")
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatalf("launch token file remains after shutdown: %v", err)
	}
}
