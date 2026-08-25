package meridian

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	integrationmanifest "sdmm/internal/aphelion/integration/manifest"
)

type staticRepositoryStateReader struct {
	state RepositoryState
	err   error
}

func (reader staticRepositoryStateReader) Read(context.Context, string) (RepositoryState, error) {
	return reader.state, reader.err
}

func TestStagerRejectsRepositoryIdentityMismatch(t *testing.T) {
	stager, candidate, manifest := newStageFixture(t)
	manifest.RepositoryIdentity = "unexpected/repository"

	_, err := stager.Stage(context.Background(), manifest, candidate)
	if err == nil || !strings.Contains(err.Error(), "repository identity") {
		t.Fatalf("Stage() error = %v, want repository identity rejection", err)
	}
}

func TestStagerRejectsDirtyOrUnexpectedRevision(t *testing.T) {
	t.Run("dirty", func(t *testing.T) {
		stager, candidate, manifest := newStageFixtureWithState(t, RepositoryState{Revision: strings.Repeat("a", 40), Dirty: true})
		_, err := stager.Stage(context.Background(), manifest, candidate)
		if err == nil || !strings.Contains(err.Error(), "dirty") {
			t.Fatalf("Stage() error = %v, want dirty repository rejection", err)
		}
	})

	t.Run("revision", func(t *testing.T) {
		stager, candidate, manifest := newStageFixtureWithState(t, RepositoryState{Revision: strings.Repeat("b", 40)})
		_, err := stager.Stage(context.Background(), manifest, candidate)
		if err == nil || !strings.Contains(err.Error(), "revision") {
			t.Fatalf("Stage() error = %v, want revision rejection", err)
		}
	})
}

func TestNewStagerRejectsTargetOutsideRepository(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tgstation.dme"), []byte("dme"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(root), "outside.dmm")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })

	_, err := NewStager(StageConfig{
		Repository: Repository{Identity: "Meridian-Rift", Root: root, DME: "tgstation.dme", Targets: map[string]string{"test": filepath.Join("..", filepath.Base(outside))}},
		StageRoot:  filepath.Join(t.TempDir(), "stage"), EnvironmentSHA256: strings.Repeat("0", 64),
		State: staticRepositoryStateReader{},
	})
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("NewStager() error = %v, want containment rejection", err)
	}
}

func TestStagerRejectsChangedInputHash(t *testing.T) {
	stager, candidate, manifest := newStageFixture(t)
	manifest.InputMapSHA256 = hashBytes([]byte("old map"))

	_, err := stager.Stage(context.Background(), manifest, candidate)
	if err == nil || !strings.Contains(err.Error(), "input map hash") {
		t.Fatalf("Stage() error = %v, want input hash rejection", err)
	}
}

func TestStagerRejectsEnvironmentHashMismatch(t *testing.T) {
	stager, candidate, manifest := newStageFixture(t)
	manifest.EnvironmentSHA256 = strings.Repeat("f", 64)

	_, err := stager.Stage(context.Background(), manifest, candidate)
	if err == nil || !strings.Contains(err.Error(), "environment hash") {
		t.Fatalf("Stage() error = %v, want environment hash rejection", err)
	}
}

func TestStagerRejectsInvalidManifest(t *testing.T) {
	stager, candidate, manifest := newStageFixture(t)
	manifest.SchemaVersion = 99

	_, err := stager.Stage(context.Background(), manifest, candidate)
	if err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("Stage() error = %v, want invalid manifest rejection", err)
	}
}

func TestStagerCreatesAtomicStage(t *testing.T) {
	stager, candidate, manifest := newStageFixture(t)

	artifact, err := stager.Stage(context.Background(), manifest, candidate)
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	got, err := os.ReadFile(artifact.MapFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(candidate) {
		t.Fatalf("staged map = %q, want %q", got, candidate)
	}
	if _, err := os.Stat(artifact.ManifestFile); err != nil {
		t.Fatalf("manifest file: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(artifact.Directory))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".stage-") {
			t.Fatalf("temporary stage remains after publication: %s", entry.Name())
		}
	}
	if _, err := stager.Stage(context.Background(), manifest, candidate); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second Stage() error = %v, want immutable-stage rejection", err)
	}
}

func newStageFixture(t *testing.T) (*Stager, []byte, integrationmanifest.Manifest) {
	t.Helper()
	return newStageFixtureWithState(t, RepositoryState{Revision: strings.Repeat("a", 40)})
}

func newStageFixtureWithState(t *testing.T, state RepositoryState) (*Stager, []byte, integrationmanifest.Manifest) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tgstation.dme"), []byte("dme"), 0o600); err != nil {
		t.Fatal(err)
	}
	current := []byte("current map")
	if err := os.MkdirAll(filepath.Join(root, "_maps"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "_maps", "test.dmm"), current, 0o600); err != nil {
		t.Fatal(err)
	}
	candidate := []byte("candidate map")
	manifest := integrationmanifest.Manifest{
		SchemaVersion: integrationmanifest.SupportedSchemaVersion, RepositoryIdentity: "Meridian-Rift", RepositoryRevision: strings.Repeat("a", 40),
		DMEIdentifier: "tgstation.dme", MapTargetID: "test-map", ProtocolVersion: integrationmanifest.SupportedProtocolVersion,
		EnvironmentSHA256: strings.Repeat("0", 64), InputMapSHA256: hashBytes(current), OutputMapSHA256: hashBytes(candidate), AcceptedRevision: 7,
		Producer: integrationmanifest.Tool{Name: "AphelionDMM", Version: "test"},
	}
	stager, err := NewStager(StageConfig{
		Repository: Repository{Identity: "Meridian-Rift", Root: root, DME: "tgstation.dme", Targets: map[string]string{"test-map": filepath.Join("_maps", "test.dmm")}},
		StageRoot:  filepath.Join(t.TempDir(), "stages"), EnvironmentSHA256: manifest.EnvironmentSHA256,
		State: staticRepositoryStateReader{state: state},
	})
	if err != nil {
		t.Fatal(err)
	}
	return stager, candidate, manifest
}

func hashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
