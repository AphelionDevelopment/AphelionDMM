package meridian

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	integrationmanifest "sdmm/internal/aphelion/integration/manifest"
)

// StageConfig is trusted local configuration for immutable map staging.
type StageConfig struct {
	Repository        Repository
	StageRoot         string
	EnvironmentSHA256 string
	AllowDirty        bool
	State             RepositoryStateReader
}

// StagedArtifact identifies one completely published immutable stage.
type StagedArtifact struct {
	Directory      string `json:"directory"`
	MapFile        string `json:"map_file"`
	ManifestFile   string `json:"manifest_file"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

// Stager verifies repository state and hashes before publishing a separate stage.
type Stager struct {
	repository        Repository
	stageRoot         string
	environmentSHA256 string
	allowDirty        bool
	state             RepositoryStateReader
}

// NewStager validates and freezes trusted repository and staging configuration.
func NewStager(config StageConfig) (*Stager, error) {
	repository, err := normalizeRepository(config.Repository)
	if err != nil {
		return nil, err
	}
	if config.StageRoot == "" {
		return nil, fmt.Errorf("stage root is required")
	}
	if !validSHA256(config.EnvironmentSHA256) {
		return nil, fmt.Errorf("trusted environment hash is invalid")
	}
	if err := os.MkdirAll(config.StageRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create stage root: %w", err)
	}
	stageRoot, err := canonicalExistingDirectory(config.StageRoot, "stage root")
	if err != nil {
		return nil, err
	}
	state := config.State
	if state == nil {
		state = GitRepositoryStateReader{}
	}
	return &Stager{repository: repository, stageRoot: stageRoot, environmentSHA256: config.EnvironmentSHA256, allowDirty: config.AllowDirty, state: state}, nil
}

// Stage publishes candidate map bytes only after manifest, revision, and hash validation.
func (stager *Stager) Stage(ctx context.Context, manifest integrationmanifest.Manifest, candidate []byte) (StagedArtifact, error) {
	if err := manifest.Validate(); err != nil {
		return StagedArtifact{}, fmt.Errorf("invalid stage manifest: %w", err)
	}
	if manifest.RepositoryIdentity != stager.repository.Identity {
		return StagedArtifact{}, fmt.Errorf("repository identity does not match trusted configuration")
	}
	if manifest.DMEIdentifier != stager.repository.DME {
		return StagedArtifact{}, fmt.Errorf("DME identifier does not match trusted configuration")
	}
	if manifest.EnvironmentSHA256 != stager.environmentSHA256 {
		return StagedArtifact{}, fmt.Errorf("environment hash does not match trusted configuration")
	}
	target, ok := stager.repository.Targets[manifest.MapTargetID]
	if !ok {
		return StagedArtifact{}, fmt.Errorf("map target identifier does not match trusted configuration")
	}
	state, err := stager.state.Read(ctx, stager.repository.Root)
	if err != nil {
		return StagedArtifact{}, err
	}
	if state.Revision != manifest.RepositoryRevision {
		return StagedArtifact{}, fmt.Errorf("repository revision does not match manifest")
	}
	if state.Dirty && !stager.allowDirty {
		return StagedArtifact{}, fmt.Errorf("repository is dirty and staging policy requires a clean checkout")
	}
	targetPath, err := resolveContained(stager.repository.Root, target)
	if err != nil {
		return StagedArtifact{}, err
	}
	current, err := os.ReadFile(targetPath)
	if err != nil {
		return StagedArtifact{}, fmt.Errorf("read input map: %w", err)
	}
	if hashData(current) != manifest.InputMapSHA256 {
		return StagedArtifact{}, fmt.Errorf("input map hash changed after manifest creation")
	}
	if hashData(candidate) != manifest.OutputMapSHA256 {
		return StagedArtifact{}, fmt.Errorf("output map hash does not match manifest")
	}
	manifestHash, err := integrationmanifest.CanonicalSHA256(manifest)
	if err != nil {
		return StagedArtifact{}, fmt.Errorf("hash stage manifest: %w", err)
	}
	finalDirectory := filepath.Join(stager.stageRoot, manifestHash)
	if _, err := os.Stat(finalDirectory); err == nil {
		return StagedArtifact{}, fmt.Errorf("immutable stage already exists")
	} else if !os.IsNotExist(err) {
		return StagedArtifact{}, fmt.Errorf("inspect stage destination: %w", err)
	}
	temporary, err := os.MkdirTemp(stager.stageRoot, ".stage-")
	if err != nil {
		return StagedArtifact{}, fmt.Errorf("create temporary stage: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(temporary)
		}
	}()
	mapFile := filepath.Join(temporary, "map.dmm")
	if err := writeSyncedFile(mapFile, candidate); err != nil {
		return StagedArtifact{}, err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return StagedArtifact{}, fmt.Errorf("encode stage manifest: %w", err)
	}
	encoded = append(encoded, '\n')
	manifestFile := filepath.Join(temporary, "manifest.json")
	if err := writeSyncedFile(manifestFile, encoded); err != nil {
		return StagedArtifact{}, err
	}
	if err := os.Rename(temporary, finalDirectory); err != nil {
		return StagedArtifact{}, fmt.Errorf("publish immutable stage: %w", err)
	}
	published = true
	return StagedArtifact{
		Directory: finalDirectory, MapFile: filepath.Join(finalDirectory, "map.dmm"),
		ManifestFile: filepath.Join(finalDirectory, "manifest.json"), ManifestSHA256: manifestHash,
	}, nil
}

func writeSyncedFile(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create staged file: %w", err)
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return fmt.Errorf("write staged file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync staged file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close staged file: %w", err)
	}
	return nil
}

func hashData(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}
