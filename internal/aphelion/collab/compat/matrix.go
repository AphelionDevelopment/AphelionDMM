package compat

import (
	"fmt"
	"slices"

	"sdmm/internal/aphelion/collab/model"
)

const CurrentRelease = "current"

type Release struct {
	Name             string   `json:"name"`
	ProtocolVersions []uint16 `json:"protocol_versions"`
	SchemaVersions   []uint16 `json:"schema_versions"`
}

type RollingPair struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Matrix struct {
	Releases     []Release     `json:"releases"`
	RollingPairs []RollingPair `json:"rolling_pairs"`
}

type Negotiated struct {
	ProtocolVersion uint16 `json:"protocol_version"`
	SchemaVersion   uint16 `json:"schema_version"`
}

func DefaultMatrix() Matrix {
	return Matrix{Releases: []Release{{
		Name:             CurrentRelease,
		ProtocolVersions: []uint16{model.ProtocolVersion},
		SchemaVersions:   []uint16{model.SchemaVersion},
	}}}
}

func (matrix Matrix) Validate() error {
	if len(matrix.Releases) == 0 {
		return fmt.Errorf("compatibility matrix has no releases")
	}
	releases := make(map[string]Release, len(matrix.Releases))
	for _, release := range matrix.Releases {
		if release.Name == "" {
			return fmt.Errorf("compatibility release has an empty name")
		}
		if _, exists := releases[release.Name]; exists {
			return fmt.Errorf("compatibility release %q is duplicated", release.Name)
		}
		if err := validateVersions(release.Name, "protocol", release.ProtocolVersions); err != nil {
			return err
		}
		if err := validateVersions(release.Name, "schema", release.SchemaVersions); err != nil {
			return err
		}
		releases[release.Name] = release
	}
	for _, pair := range matrix.RollingPairs {
		if _, exists := releases[pair.From]; !exists {
			return fmt.Errorf("rolling pair references unknown release %q", pair.From)
		}
		if _, exists := releases[pair.To]; !exists {
			return fmt.Errorf("rolling pair references unknown release %q", pair.To)
		}
	}
	return nil
}

func (matrix Matrix) Negotiate(protocolVersion, schemaVersion uint16) (Negotiated, error) {
	if err := matrix.Validate(); err != nil {
		return Negotiated{}, err
	}
	release, found := matrix.release(CurrentRelease)
	if !found {
		return Negotiated{}, fmt.Errorf("compatibility matrix has no %q release", CurrentRelease)
	}
	if !slices.Contains(release.ProtocolVersions, protocolVersion) {
		return Negotiated{}, fmt.Errorf("protocol version %d is unsupported", protocolVersion)
	}
	if !slices.Contains(release.SchemaVersions, schemaVersion) {
		return Negotiated{}, fmt.Errorf("schema version %d is unsupported", schemaVersion)
	}
	return Negotiated{ProtocolVersion: protocolVersion, SchemaVersion: schemaVersion}, nil
}

func (matrix Matrix) ValidateRolling(from, to string) error {
	if err := matrix.Validate(); err != nil {
		return err
	}
	fromRelease, fromFound := matrix.release(from)
	toRelease, toFound := matrix.release(to)
	if !fromFound || !toFound {
		return fmt.Errorf("rolling releases %q and %q must both exist", from, to)
	}
	if from != to && !slices.Contains(matrix.RollingPairs, RollingPair{From: from, To: to}) {
		return fmt.Errorf("rolling pair %q to %q is not declared", from, to)
	}
	if !overlaps(fromRelease.ProtocolVersions, toRelease.ProtocolVersions) {
		return fmt.Errorf("rolling pair %q to %q has no protocol overlap", from, to)
	}
	if !overlaps(fromRelease.SchemaVersions, toRelease.SchemaVersions) {
		return fmt.Errorf("rolling pair %q to %q has no schema overlap", from, to)
	}
	return nil
}

func (matrix Matrix) release(name string) (Release, bool) {
	for _, release := range matrix.Releases {
		if release.Name == name {
			return release, true
		}
	}
	return Release{}, false
}

func validateVersions(release, kind string, versions []uint16) error {
	if len(versions) == 0 {
		return fmt.Errorf("compatibility release %q has no %s versions", release, kind)
	}
	seen := make(map[uint16]struct{}, len(versions))
	for _, version := range versions {
		if version == 0 {
			return fmt.Errorf("compatibility release %q has invalid %s version zero", release, kind)
		}
		if _, exists := seen[version]; exists {
			return fmt.Errorf("compatibility release %q duplicates %s version %d", release, kind, version)
		}
		seen[version] = struct{}{}
	}
	return nil
}

func overlaps(first, second []uint16) bool {
	for _, version := range first {
		if slices.Contains(second, version) {
			return true
		}
	}
	return false
}
