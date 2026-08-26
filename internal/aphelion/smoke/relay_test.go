package smoke

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShippedRelayCommandRoutesOpaqueTraffic(t *testing.T) {
	if testing.Short() {
		t.Skip("shipped relay command smoke is an integration test")
	}
	repositoryRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	executableName := "apheliondmm-relay"
	if runtime.GOOS == "windows" {
		executableName += ".exe"
	}
	executable := filepath.Join(t.TempDir(), executableName)
	build := exec.Command("go", "build", "-trimpath", "-o", executable, "./cmd/apheliondmm-relay")
	build.Dir = repositoryRoot
	buildOutput, err := build.CombinedOutput()
	require.NoError(t, err, string(buildOutput))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	configPath := filepath.Join(t.TempDir(), "relay.yaml")
	config := fmt.Sprintf(`version: 1
public_origin: https://mapping.a13.info
bind_address: %s
trusted_proxy_cidrs:
  - 127.0.0.1/32
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
  metrics_bind_address: 127.0.0.1:0
`, address)
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))
	endpoint := "http://" + address
	runCycle := func() string {
		ctx, cancel := context.WithCancel(context.Background())
		command := exec.CommandContext(ctx, executable, "-config", configPath)
		var output bytes.Buffer
		command.Stdout = &output
		command.Stderr = &output
		require.NoError(t, command.Start())
		require.Eventually(t, func() bool {
			response, requestErr := http.Get(endpoint + "/v1/health/ready")
			if requestErr != nil {
				return false
			}
			defer response.Body.Close()
			return response.StatusCode == http.StatusOK
		}, 10*time.Second, 25*time.Millisecond, "relay did not become ready")
		smokeContext, cancelSmoke := context.WithTimeout(context.Background(), 10*time.Second)
		require.NoError(t, RunRelayTransport(smokeContext, endpoint))
		cancelSmoke()
		cancel()
		waited := make(chan struct{})
		go func() {
			_ = command.Wait()
			close(waited)
		}()
		select {
		case <-waited:
		case <-time.After(5 * time.Second):
			t.Fatal("relay command did not stop after cancellation")
		}
		return output.String()
	}
	firstOutput := runCycle()
	secondOutput := runCycle()
	require.NotContains(t, strings.ToLower(firstOutput+secondOutput), "relay smoke editor")
}
