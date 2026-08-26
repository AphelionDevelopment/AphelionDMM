package relay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfigAcceptsStrictSingleFile(t *testing.T) {
	config, err := LoadConfig(writeConfig(t, validConfigYAML))
	require.NoError(t, err)
	require.Equal(t, "https://mapping.a13.info", config.PublicOrigin)
	require.Equal(t, 1000, config.Limits.MaxRooms)
	require.Equal(t, "127.0.0.1:9090", config.Observability.MetricsBindAddress)
}

func TestLoadConfigRejectsUnknownOrServerAuthoritySettings(t *testing.T) {
	for name, addition := range map[string]string{
		"unknown":  "mystery: true\n",
		"database": "database_url: postgres://example.invalid/db\n",
		"oidc":     "oidc_issuer: https://example.invalid\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, validConfigYAML+addition))
			require.Error(t, err)
		})
	}
}

func TestLoadConfigRejectsCleartextPublicOriginAndUnsafeLimits(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, replaceConfig(validConfigYAML, "https://mapping.a13.info", "http://mapping.a13.info")))
	require.ErrorContains(t, err, "HTTPS")
	_, err = LoadConfig(writeConfig(t, replaceConfig(validConfigYAML, "max_connections_per_room: 32", "max_connections_per_room: 0")))
	require.ErrorContains(t, err, "limits")
}

func writeConfig(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "relay.yaml")
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
	return path
}

func replaceConfig(value, old, replacement string) string {
	return strings.Replace(value, old, replacement, 1)
}

const validConfigYAML = `version: 1
public_origin: https://mapping.a13.info
bind_address: 127.0.0.1:8080
trusted_proxy_cidrs:
  - 127.0.0.1/32
room_idle_ttl: 30m
limits:
  max_rooms: 1000
  max_connections: 4000
  max_connections_per_room: 32
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
