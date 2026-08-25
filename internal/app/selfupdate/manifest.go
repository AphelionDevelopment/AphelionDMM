package selfupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"aead.dev/minisign"

	"sdmm/internal/env"
	"sdmm/internal/req"
)

const (
	placeholderVersion  = "%VERSION%"
	maxManifestBytes    = 1 << 20
	maxSignatureBytes   = 4 << 10
	manifestContentType = "application/json"
)

// APHELION EDIT ADDITION START - SECURE_UPDATER
// TrustedPublicKey is the minisign trust root injected by the approved release pipeline.
// An empty value intentionally disables update discovery rather than accepting unsigned metadata.
var TrustedPublicKey string

type Artifact struct {
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}

type Manifest struct {
	Name          string        `json:"name"`
	Version       string        `json:"version"`
	Description   string        `json:"description"`
	DownloadLinks DownloadLinks `json:"downloadLinks"`
}

type DownloadLinks struct {
	Windows Artifact `json:"windows"`
	Linux   Artifact `json:"linux"`
	MacOS   Artifact `json:"macOS"`
}

type signedManifest struct {
	Manifest  json.RawMessage `json:"manifest"`
	Signature string          `json:"signature"`
}

func ParseManifest(data []byte) (Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode update manifest: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return Manifest{}, err
	}
	replaceVersionPlaceholder(&manifest.DownloadLinks.Windows.URL, manifest.Version)
	replaceVersionPlaceholder(&manifest.DownloadLinks.Linux.URL, manifest.Version)
	replaceVersionPlaceholder(&manifest.DownloadLinks.MacOS.URL, manifest.Version)
	if err := manifest.validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func ParseSignedManifest(data []byte, publicKeyText string) (Manifest, error) {
	publicKey, err := parsePublicKey(publicKeyText)
	if err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var envelope signedManifest
	if err := decoder.Decode(&envelope); err != nil {
		return Manifest{}, fmt.Errorf("decode signed update manifest: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return Manifest{}, err
	}
	if len(envelope.Manifest) == 0 || len(envelope.Signature) == 0 || len(envelope.Signature) > maxSignatureBytes {
		return Manifest{}, fmt.Errorf("signed update manifest is incomplete")
	}
	if !minisign.Verify(publicKey, envelope.Manifest, []byte(envelope.Signature)) {
		return Manifest{}, fmt.Errorf("update manifest signature verification failed")
	}
	return ParseManifest(envelope.Manifest)
}

func FetchRemoteManifest() (Manifest, error) {
	if TrustedPublicKey == "" {
		return Manifest{}, fmt.Errorf("update signing key is not configured")
	}
	return FetchRemoteManifestWithClient(context.Background(), req.NewClient(), env.Manifest, TrustedPublicKey)
}

func FetchRemoteManifestWithClient(ctx context.Context, client *http.Client, manifestURL, publicKeyText string) (Manifest, error) {
	manifestData, err := req.GetWithClient(ctx, client, manifestURL, req.Options{
		MaxBytes:     maxManifestBytes,
		ContentTypes: []string{manifestContentType, "application/manifest+json"},
	})
	if err != nil {
		return Manifest{}, fmt.Errorf("unable to get manifest data: %w", err)
	}
	return ParseSignedManifest(manifestData, publicKeyText)
}

func (manifest Manifest) validate() error {
	if manifest.Name == "" || manifest.Version == "" {
		return fmt.Errorf("update manifest name and version are required")
	}
	for platform, artifact := range map[string]Artifact{
		"windows": manifest.DownloadLinks.Windows,
		"linux":   manifest.DownloadLinks.Linux,
		"macOS":   manifest.DownloadLinks.MacOS,
	} {
		if err := artifact.validate(); err != nil {
			return fmt.Errorf("validate %s update artifact: %w", platform, err)
		}
	}
	return nil
}

func (artifact Artifact) validate() error {
	parsed, err := url.Parse(artifact.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("artifact URL must be absolute HTTPS")
	}
	if len(artifact.SHA256) != 64 || strings.ToLower(artifact.SHA256) != artifact.SHA256 {
		return fmt.Errorf("artifact SHA-256 must be 64 lowercase hexadecimal characters")
	}
	for _, character := range artifact.SHA256 {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return fmt.Errorf("artifact SHA-256 must be 64 lowercase hexadecimal characters")
		}
	}
	if artifact.Signature == "" || len(artifact.Signature) > maxSignatureBytes {
		return fmt.Errorf("artifact signature is missing or oversized")
	}
	return nil
}

func parsePublicKey(text string) (minisign.PublicKey, error) {
	if text == "" {
		return minisign.PublicKey{}, fmt.Errorf("update signing key is not configured")
	}
	var publicKey minisign.PublicKey
	if err := publicKey.UnmarshalText([]byte(text)); err != nil {
		return minisign.PublicKey{}, fmt.Errorf("parse update signing key: %w", err)
	}
	return publicKey, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode update manifest trailing data: %w", err)
	}
	return fmt.Errorf("decode update manifest: trailing JSON value")
}

func replaceVersionPlaceholder(value *string, version string) {
	*value = strings.ReplaceAll(*value, placeholderVersion, version)
}

// APHELION EDIT ADDITION END

/* APHELION EDIT REMOVAL START - SECURE_UPDATER
type Manifest struct {
	Name          string        `json:"name"`
	Version       string        `json:"version"`
	Description   string        `json:"description"`
	DownloadLinks DownloadLinks `json:"downloadLinks"`
}

type DownloadLinks struct {
	Windows string `json:"windows"`
	Linux   string `json:"linux"`
	MacOS   string `json:"macOS"`
}

func ParseManifest(data []byte) (manifest Manifest, err error) {
	if err = json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	replaceVersionPlaceholder(&manifest.DownloadLinks.Windows, manifest.Version)
	replaceVersionPlaceholder(&manifest.DownloadLinks.Linux, manifest.Version)
	replaceVersionPlaceholder(&manifest.DownloadLinks.MacOS, manifest.Version)
	return manifest, nil
}

func FetchRemoteManifest() (Manifest, error) {
	if manifestData, err := req.Get(env.Manifest); err == nil {
		return ParseManifest(manifestData)
	} else {
		return Manifest{}, fmt.Errorf("unable to get manifest data: %w", err)
	}
}
APHELION EDIT REMOVAL END */
