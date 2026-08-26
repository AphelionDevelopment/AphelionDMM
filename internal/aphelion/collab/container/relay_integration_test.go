//go:build container

package container_test

import (
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

	"sdmm/internal/aphelion/smoke"
)

func TestRelayImageLifecycle(t *testing.T) {
	image := os.Getenv("APHELION_RELAY_TEST_IMAGE")
	if image == "" {
		t.Skip("APHELION_RELAY_TEST_IMAGE is not set")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("locate docker: %v", err)
	}
	fixtureID := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	name := "aphelion-relay-" + fixtureID
	port := relayAvailablePort(t)
	_, sourceFile, _, ok := runtimeCaller()
	if !ok {
		t.Fatal("locate relay container test source")
	}
	configPath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "..", "deploy", "relay", "relay.yaml.example")
	relayDocker(t, "run", "--detach", "--name", name,
		"--publish", fmt.Sprintf("127.0.0.1:%d:8080", port),
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--mount", fmt.Sprintf("type=bind,source=%s,target=/etc/apheliondmm/relay.yaml,readonly", configPath),
		image)
	t.Cleanup(func() { relayDockerCleanup(name) })

	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
	relayWaitHTTP(t, endpoint+"/v1/health/ready", time.Minute)
	relayAssertHardened(t, name)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err := smoke.RunRelayTransport(ctx, endpoint); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()

	relayDocker(t, "restart", "--timeout", "10", name)
	relayWaitHTTP(t, endpoint+"/v1/health/ready", time.Minute)
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	if err := smoke.RunRelayTransport(ctx, endpoint); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	logs := strings.ToLower(relayDocker(t, "logs", name))
	if strings.Contains(logs, "relay smoke editor") {
		t.Fatal("relay container logs contain decrypted application plaintext")
	}
}

func runtimeCaller() (uintptr, string, int, bool) {
	return runtime.Caller(0)
}

func relayAvailablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func relayWaitHTTP(t *testing.T, target string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		response, err := (&http.Client{Timeout: 2 * time.Second}).Get(target)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("%s did not become ready", target)
}

func relayAssertHardened(t *testing.T, name string) {
	t.Helper()
	inspection := strings.TrimSpace(relayDocker(t, "inspect", "--format", "{{.HostConfig.ReadonlyRootfs}}|{{.Config.User}}|{{json .HostConfig.CapDrop}}|{{json .HostConfig.SecurityOpt}}|{{json .Mounts}}", name))
	if !strings.HasPrefix(inspection, `true|nonroot:nonroot|["ALL"]|["no-new-privileges:true"]|`) {
		t.Fatalf("relay runtime hardening = %s", inspection)
	}
	if strings.Contains(strings.ToLower(inspection), "volume") {
		t.Fatalf("relay runtime has a persistent volume: %s", inspection)
	}
}

func relayDocker(t *testing.T, arguments ...string) string {
	t.Helper()
	command := exec.Command("docker", arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func relayDockerCleanup(name string) {
	command := exec.Command("docker", "rm", "--force", name)
	_, _ = command.CombinedOutput()
}
