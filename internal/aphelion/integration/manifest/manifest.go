package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const (
	SupportedSchemaVersion   uint16 = 1
	SupportedProtocolVersion uint16 = 1
	maxIdentifierLength             = 256
	maxToolValueLength              = 128
)

var (
	sha256Pattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	revisionPattern = regexp.MustCompile(`^[0-9a-f]{40,64}$`)
	identifierPart  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// Tool identifies the producer of an integration artifact.
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Manifest binds a staged map artifact to its repository and compatibility inputs.
type Manifest struct {
	SchemaVersion      uint16 `json:"schema_version"`
	RepositoryIdentity string `json:"repository_identity"`
	RepositoryRevision string `json:"repository_revision"`
	DMEIdentifier      string `json:"dme_identifier"`
	MapTargetID        string `json:"map_target_id"`
	ProtocolVersion    uint16 `json:"protocol_version"`
	EnvironmentSHA256  string `json:"environment_sha256"`
	InputMapSHA256     string `json:"input_map_sha256"`
	OutputMapSHA256    string `json:"output_map_sha256"`
	AcceptedRevision   uint64 `json:"accepted_revision"`
	ContentSHA256      string `json:"content_manifest_sha256,omitempty"`
	Producer           Tool   `json:"producer"`
}

// Decode strictly decodes and validates one compatibility manifest.
func Decode(reader io.Reader) (Manifest, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var result Manifest
	if err := decoder.Decode(&result); err != nil {
		return Manifest{}, fmt.Errorf("decode integration manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Manifest{}, err
	}
	if err := result.Validate(); err != nil {
		return Manifest{}, err
	}
	return result, nil
}

// Validate enforces the supported compatibility versions and bounded logical values.
func (manifest Manifest) Validate() error {
	if manifest.SchemaVersion != SupportedSchemaVersion {
		return fmt.Errorf("schema_version %d is unsupported", manifest.SchemaVersion)
	}
	if manifest.ProtocolVersion != SupportedProtocolVersion {
		return fmt.Errorf("protocol_version %d is unsupported", manifest.ProtocolVersion)
	}
	for name, value := range map[string]string{
		"repository_identity": manifest.RepositoryIdentity,
		"dme_identifier":      manifest.DMEIdentifier,
		"map_target_id":       manifest.MapTargetID,
	} {
		if err := validateLogicalIdentifier(name, value); err != nil {
			return err
		}
	}
	if !revisionPattern.MatchString(manifest.RepositoryRevision) {
		return fmt.Errorf("repository_revision must be 40 to 64 lowercase hexadecimal characters")
	}
	for name, value := range map[string]string{
		"environment_sha256":      manifest.EnvironmentSHA256,
		"input_map_sha256":        manifest.InputMapSHA256,
		"output_map_sha256":       manifest.OutputMapSHA256,
		"content_manifest_sha256": manifest.ContentSHA256,
	} {
		if value == "" && name == "content_manifest_sha256" {
			continue
		}
		if !sha256Pattern.MatchString(value) {
			return fmt.Errorf("%s must be 64 lowercase hexadecimal characters", name)
		}
	}
	if err := validateToolValue("producer.name", manifest.Producer.Name); err != nil {
		return err
	}
	if err := validateToolValue("producer.version", manifest.Producer.Version); err != nil {
		return err
	}
	return nil
}

// CanonicalSHA256 returns the SHA-256 of the manifest's compact field-ordered JSON form.
func CanonicalSHA256(manifest Manifest) (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("encode canonical integration manifest: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode integration manifest: trailing JSON value")
		}
		return fmt.Errorf("decode integration manifest: %w", err)
	}
	return nil
}

func validateLogicalIdentifier(name, value string) error {
	if value == "" || len(value) > maxIdentifierLength {
		return fmt.Errorf("%s must contain 1 to %d characters", name, maxIdentifierLength)
	}
	if strings.Contains(value, `\`) || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return fmt.Errorf("%s must be a logical identifier, not a filesystem path", name)
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || !identifierPart.MatchString(part) {
			return fmt.Errorf("%s contains an invalid identifier segment", name)
		}
	}
	return nil
}

func validateToolValue(name, value string) error {
	if value == "" || len(value) > maxToolValueLength {
		return fmt.Errorf("%s must contain 1 to %d characters", name, maxToolValueLength)
	}
	return nil
}
