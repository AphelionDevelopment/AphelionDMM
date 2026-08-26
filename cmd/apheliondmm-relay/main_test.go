package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunRequiresConfigAndRejectsPositionalArguments(t *testing.T) {
	require.ErrorContains(t, run(context.Background(), nil, &bytes.Buffer{}), "-config")
	require.Error(t, run(context.Background(), []string{"extra"}, &bytes.Buffer{}))
}

func TestRunLoadsConfigBeforeListening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.yaml")
	require.NoError(t, os.WriteFile(path, []byte("version: 999\n"), 0o600))
	err := run(context.Background(), []string{"-config", path}, &bytes.Buffer{})
	require.ErrorContains(t, err, "version")
}

func TestRunCanValidateConfigWithoutListening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.yaml")
	require.NoError(t, os.WriteFile(path, []byte(validRelayConfig), 0o600))
	var output bytes.Buffer
	require.NoError(t, run(context.Background(), []string{"-config", path, "-check-config"}, &output))
	require.Contains(t, output.String(), "configuration valid")
}

func TestRunRequiresCloudflaredAndTokenFileTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.yaml")
	require.NoError(t, os.WriteFile(path, []byte(validRelayConfig), 0o600))
	require.ErrorContains(t, run(context.Background(), []string{"-config", path, "-cloudflared", "cloudflared.exe"}, &bytes.Buffer{}), "tunnel-token-file")
	require.ErrorContains(t, run(context.Background(), []string{"-config", path, "-tunnel-token-file", "token.txt"}, &bytes.Buffer{}), "cloudflared")
}

const validRelayConfig = `version: 1
public_origin: https://mapping.a13.info
bind_address: 127.0.0.1:8080
trusted_proxy_cidrs: []
room_idle_ttl: 30m
limits:
  max_rooms: 10
  max_connections: 20
  max_connections_per_room: 8
  max_message_bytes: 1048576
  max_room_bytes_per_second: 8388608
  connect_burst: 30
  connect_window: 1m
  message_burst: 240
  message_window: 1s
observability:
  log_level: info
  metrics_bind_address: 127.0.0.1:9090
`
