package container_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"sdmm/internal/aphelion/collab/relay"
)

func TestRelayDeploymentIsStatelessPrivateAndHardened(t *testing.T) {
	root := relayDeploymentRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Services map[string]struct {
			Image       string         `yaml:"image"`
			Ports       []string       `yaml:"ports"`
			ReadOnly    bool           `yaml:"read_only"`
			SecurityOpt []string       `yaml:"security_opt"`
			CapDrop     []string       `yaml:"cap_drop"`
			Healthcheck map[string]any `yaml:"healthcheck"`
			Volumes     []string       `yaml:"volumes"`
			Secrets     []any          `yaml:"secrets"`
		} `yaml:"services"`
		Volumes map[string]any `yaml:"volumes"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Services) != 1 {
		t.Fatalf("base relay Compose services = %d, want 1", len(document.Services))
	}
	service, ok := document.Services["relay"]
	if !ok || service.Image == "" || len(service.Ports) != 0 || !service.ReadOnly || !relayContains(service.SecurityOpt, "no-new-privileges:true") || !relayContains(service.CapDrop, "ALL") || len(service.Healthcheck) == 0 || len(service.Volumes) != 0 || len(service.Secrets) != 0 {
		t.Fatalf("relay service is not private, stateless, and hardened: %#v", service)
	}
	if len(document.Volumes) != 0 {
		t.Fatalf("relay Compose declares persistent volumes: %#v", document.Volumes)
	}
	text := strings.ToLower(string(data))
	for _, forbidden := range []string{"postgres", "oidc", "database", "backup", "/maps", "docker.sock"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("relay Compose contains forbidden server-authority value %q", forbidden)
		}
	}
}

func TestRelayConfigAndOverlaysMatchPublicTopology(t *testing.T) {
	root := relayDeploymentRoot(t)
	config, err := relay.LoadConfig(filepath.Join(root, "relay.yaml.example"))
	if err != nil {
		t.Fatal(err)
	}
	if config.PublicOrigin != "https://mapping.a13.info" || config.BindAddress != "0.0.0.0:8080" {
		t.Fatalf("relay example config = %#v", config)
	}
	cloudflare, err := os.ReadFile(filepath.Join(root, "compose.cloudflare.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cloudflareText := strings.ToLower(string(cloudflare))
	for _, required := range []string{"cloudflare/cloudflared:", "@sha256:", "token-file", "cloudflare_tunnel_token", "no-autoupdate", "cap_drop", "read_only"} {
		if !strings.Contains(cloudflareText, required) {
			t.Fatalf("Cloudflare overlay is missing %q", required)
		}
	}
	for _, forbidden := range []string{"access:", "oidc", "postgres", "database"} {
		if strings.Contains(cloudflareText, forbidden) {
			t.Fatalf("Cloudflare overlay contains forbidden value %q", forbidden)
		}
	}
	loopback, err := os.ReadFile(filepath.Join(root, "compose.loopback.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loopback), "127.0.0.1:8080:8080") {
		t.Fatal("loopback overlay does not bind relay only to loopback")
	}
}

func TestRelayDockerfileAndOperatorSurfaceStayMinimal(t *testing.T) {
	root := relayDeploymentRoot(t)
	dockerfile, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	dockerText := string(dockerfile)
	for _, required := range []string{"CGO_ENABLED=0", "apheliondmm-relay", "apheliondmm-healthcheck", "distroless", "USER nonroot:nonroot", "HEALTHCHECK"} {
		if !strings.Contains(dockerText, required) {
			t.Fatalf("relay Dockerfile is missing %q", required)
		}
	}
	if strings.Contains(strings.ToLower(dockerText), "apheliondmm-hosted") {
		t.Fatal("relay Dockerfile builds the legacy hosted service")
	}
	operations, err := os.ReadFile(filepath.Join(root, "operations.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	operationsText := string(operations)
	for _, action := range []string{"Setup", "Validate", "Build", "StartCloudflare", "StartLoopback", "Status", "Logs", "Update", "Stop"} {
		if !strings.Contains(operationsText, "'"+action+"'") {
			t.Fatalf("relay operator script is missing %s", action)
		}
	}
	for _, forbidden := range []string{"'Backup'", "'Restore'", "'Migrate'"} {
		if strings.Contains(operationsText, forbidden) {
			t.Fatalf("relay operator script contains forbidden action %s", forbidden)
		}
	}
	if !strings.Contains(operationsText, "mapping.a13.info -> http://relay:8080") {
		t.Fatal("relay operator script does not state the public mapping")
	}
}

func relayDeploymentRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate relay configuration test source")
	}
	return filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "..", "deploy", "relay")
}

func relayContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
