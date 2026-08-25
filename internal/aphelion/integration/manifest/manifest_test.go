package manifest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestDecodeValidManifest(t *testing.T) {
	data := readFixture(t, "valid.json")

	got, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got.RepositoryIdentity != "meridian-rift" {
		t.Fatalf("RepositoryIdentity = %q, want %q", got.RepositoryIdentity, "meridian-rift")
	}
	if got.AcceptedRevision != 42 {
		t.Fatalf("AcceptedRevision = %d, want 42", got.AcceptedRevision)
	}
}

func TestDecodeRejectsIncompatibleManifest(t *testing.T) {
	_, err := Decode(bytes.NewReader(readFixture(t, "incompatible.json")))
	if err == nil {
		t.Fatal("Decode() error = nil, want unsupported schema error")
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	data := readFixture(t, "valid.json")
	data = bytes.Replace(data, []byte(`"schema_version": 1,`), []byte(`"schema_version": 1, "local_path": "C:/secret",`), 1)

	_, err := Decode(bytes.NewReader(data))
	if err == nil {
		t.Fatal("Decode() error = nil, want unknown field error")
	}
}

func TestDecodeRejectsInvalidValues(t *testing.T) {
	valid := readFixture(t, "valid.json")
	tests := map[string][]byte{
		"uppercase hash":  bytes.Replace(valid, []byte(`"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`), []byte(`"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"`), 1),
		"path identifier": bytes.Replace(valid, []byte(`"maps/meridian.dmm"`), []byte(`"C:/maps/meridian.dmm"`), 1),
		"empty producer":  bytes.Replace(valid, []byte(`"apheliondmm"`), []byte(`""`), 1),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(bytes.NewReader(data)); err == nil {
				t.Fatal("Decode() error = nil, want validation error")
			}
		})
	}
}

func TestCanonicalSHA256IsStableAcrossFormatting(t *testing.T) {
	manifest, err := Decode(bytes.NewReader(readFixture(t, "valid.json")))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	compact, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	redecoded, err := Decode(bytes.NewReader(compact))
	if err != nil {
		t.Fatalf("Decode(compact) error = %v", err)
	}

	first, err := CanonicalSHA256(manifest)
	if err != nil {
		t.Fatalf("CanonicalSHA256() error = %v", err)
	}
	second, err := CanonicalSHA256(redecoded)
	if err != nil {
		t.Fatalf("CanonicalSHA256(redecoded) error = %v", err)
	}
	if first != second {
		t.Fatalf("canonical hashes differ: %q != %q", first, second)
	}
	if first != "6f07d70bcd09f2365524ff9d21f8356f85f2391a904f199b3032360cff6fc838" {
		t.Fatalf("CanonicalSHA256() = %q, want fixture hash", first)
	}
}

func TestFixturesMatchJSONSchema(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "..", "..", "api", "integration", "aphelion-manifest.schema.json")
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	var valid any
	if err := json.Unmarshal(readFixture(t, "valid.json"), &valid); err != nil {
		t.Fatalf("json.Unmarshal(valid) error = %v", err)
	}
	if err := schema.Validate(valid); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}

	var incompatible any
	if err := json.Unmarshal(readFixture(t, "incompatible.json"), &incompatible); err != nil {
		t.Fatalf("json.Unmarshal(incompatible) error = %v", err)
	}
	if err := schema.Validate(incompatible); err == nil {
		t.Fatal("Validate(incompatible) error = nil, want incompatibility")
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", name, err)
	}
	return data
}
