package container_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestProductionComposeIsSingleReplicaAndPrivate(t *testing.T) {
	productionRoot := productionDeploymentRoot(t)
	contents, err := os.ReadFile(filepath.Join(productionRoot, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Services map[string]struct {
			User        string            `yaml:"user"`
			Ports       []string          `yaml:"ports"`
			ReadOnly    bool              `yaml:"read_only"`
			SecurityOpt []string          `yaml:"security_opt"`
			CapDrop     []string          `yaml:"cap_drop"`
			Deploy      map[string]any    `yaml:"deploy"`
			Healthcheck map[string]any    `yaml:"healthcheck"`
			Volumes     []string          `yaml:"volumes"`
			Secrets     []any             `yaml:"secrets"`
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
		Volumes map[string]any `yaml:"volumes"`
	}
	if err := yaml.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	hosted, ok := document.Services["hosted"]
	if !ok || hosted.User != "${APHELIONDMM_RUNTIME_UID:-65532}:${APHELIONDMM_RUNTIME_GID:-65532}" || len(hosted.Ports) != 0 || !hosted.ReadOnly || !contains(hosted.SecurityOpt, "no-new-privileges:true") || !contains(hosted.CapDrop, "ALL") || len(hosted.Healthcheck) == 0 || len(hosted.Secrets) < 2 {
		t.Fatalf("hosted service is not private and hardened: %#v", hosted)
	}
	postgres, ok := document.Services["postgres"]
	if !ok || len(postgres.Ports) != 0 || len(postgres.Healthcheck) == 0 || len(postgres.Volumes) == 0 || len(postgres.Secrets) == 0 {
		t.Fatalf("PostgreSQL service is not private and persistent: %#v", postgres)
	}
	if _, ok := document.Volumes["postgres-data"]; !ok {
		t.Fatal("production Compose has no named PostgreSQL volume")
	}
	text := string(contents)
	for _, forbidden := range []string{"POSTGRES_PASSWORD:", "APHELIONDMM_DATABASE_DSN:", "APHELIONDMM_OIDC_CLIENT_SECRET:", "replicas: 2"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("production Compose contains forbidden value %q", forbidden)
		}
	}
}

func TestProductionHostedConfigAndSchemaParse(t *testing.T) {
	productionRoot := productionDeploymentRoot(t)
	configData, err := os.ReadFile(filepath.Join(productionRoot, "config.yaml.example"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		PublicOrigin string `yaml:"public_origin"`
		Database     struct {
			DSN struct {
				File string `yaml:"file"`
			} `yaml:"dsn"`
		} `yaml:"database"`
		OIDC struct {
			RedirectURL  string `yaml:"redirect_url"`
			ClientSecret struct {
				File string `yaml:"file"`
			} `yaml:"client_secret"`
		} `yaml:"oidc"`
	}
	if err := yaml.Unmarshal(configData, &config); err != nil {
		t.Fatal(err)
	}
	if config.PublicOrigin != "https://mapping.a13.info" || config.OIDC.RedirectURL != "https://mapping.a13.info/v1/auth/complete" || config.Database.DSN.File != "/run/secrets/database_dsn" || config.OIDC.ClientSecret.File != "/run/secrets/oidc_client_secret" {
		t.Fatalf("production hosted config = %#v", config)
	}
	schemaPath := filepath.Join(productionRoot, "..", "..", "docs", "hosting", "hosted-config.schema.json")
	schema, err := os.Open(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = schema.Close() }()
	var schemaDocument map[string]any
	if err := json.NewDecoder(schema).Decode(&schemaDocument); err != nil {
		t.Fatal(err)
	}
	if schemaDocument["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("schema declaration = %#v", schemaDocument["$schema"])
	}
}

func TestProductionEdgeOverlaysRemainOptionalAndPublic(t *testing.T) {
	productionRoot := productionDeploymentRoot(t)
	cloudflare, err := os.ReadFile(filepath.Join(productionRoot, "compose.cloudflare.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cloudflareText := string(cloudflare)
	for _, required := range []string{"cloudflared", "token-file", "cloudflare_tunnel_token", "no-autoupdate", "APHELIONDMM_RUNTIME_UID", "APHELIONDMM_RUNTIME_GID"} {
		if !strings.Contains(cloudflareText, required) {
			t.Fatalf("Cloudflare overlay is missing %q", required)
		}
	}
	for _, forbidden := range []string{"access:", "allowed_idps", "email_domain"} {
		if strings.Contains(strings.ToLower(cloudflareText), forbidden) {
			t.Fatalf("Cloudflare overlay contains an Access gate %q", forbidden)
		}
	}
	loopback, err := os.ReadFile(filepath.Join(productionRoot, "compose.loopback.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loopback), `127.0.0.1:8080:8080`) {
		t.Fatal("reverse-proxy overlay does not bind hosted service to loopback")
	}
}

func productionDeploymentRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate production configuration test source")
	}
	return filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "..", "deploy", "production")
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
