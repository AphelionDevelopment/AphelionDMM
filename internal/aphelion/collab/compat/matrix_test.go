package compat

import (
	"testing"

	"sdmm/internal/aphelion/collab/model"
)

func TestDefaultMatrixAcceptsCurrentClient(t *testing.T) {
	negotiated, err := DefaultMatrix().Negotiate(model.ProtocolVersion, model.SchemaVersion)
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	if negotiated.ProtocolVersion != model.ProtocolVersion || negotiated.SchemaVersion != model.SchemaVersion {
		t.Fatalf("Negotiate() = %#v", negotiated)
	}
}

func TestMatrixRejectsUnsupportedProtocolBeforeJoin(t *testing.T) {
	_, err := DefaultMatrix().Negotiate(model.ProtocolVersion+1, model.SchemaVersion)
	if err == nil {
		t.Fatal("Negotiate() error = nil, want unsupported protocol")
	}
}

func TestMatrixPermitsOnlyDeclaredRollingPairs(t *testing.T) {
	matrix := Matrix{
		Releases: []Release{
			{Name: "previous", ProtocolVersions: []uint16{1}, SchemaVersions: []uint16{1}},
			{Name: "current", ProtocolVersions: []uint16{1, 2}, SchemaVersions: []uint16{1, 2}},
		},
		RollingPairs: []RollingPair{{From: "previous", To: "current"}, {From: "current", To: "previous"}},
	}
	for _, pair := range []RollingPair{{From: "previous", To: "current"}, {From: "current", To: "previous"}, {From: "current", To: "current"}} {
		if err := matrix.ValidateRolling(pair.From, pair.To); err != nil {
			t.Errorf("ValidateRolling(%q, %q) error = %v", pair.From, pair.To, err)
		}
	}
	if err := matrix.ValidateRolling("previous", "missing"); err == nil {
		t.Fatal("ValidateRolling() error = nil for missing release")
	}
}

func TestMatrixRejectsRollingPairWithoutProtocolOrSchemaOverlap(t *testing.T) {
	matrix := Matrix{
		Releases: []Release{
			{Name: "old", ProtocolVersions: []uint16{1}, SchemaVersions: []uint16{1}},
			{Name: "new-protocol", ProtocolVersions: []uint16{2}, SchemaVersions: []uint16{1}},
			{Name: "new-schema", ProtocolVersions: []uint16{1}, SchemaVersions: []uint16{2}},
		},
		RollingPairs: []RollingPair{{From: "old", To: "new-protocol"}, {From: "old", To: "new-schema"}},
	}
	if err := matrix.ValidateRolling("old", "new-protocol"); err == nil {
		t.Fatal("ValidateRolling() error = nil without protocol overlap")
	}
	if err := matrix.ValidateRolling("old", "new-schema"); err == nil {
		t.Fatal("ValidateRolling() error = nil without schema overlap")
	}
}

func TestMatrixRejectsInvalidOrAmbiguousDeclarations(t *testing.T) {
	tests := []Matrix{
		{},
		{Releases: []Release{{Name: "duplicate", ProtocolVersions: []uint16{1}, SchemaVersions: []uint16{1}}, {Name: "duplicate", ProtocolVersions: []uint16{1}, SchemaVersions: []uint16{1}}}},
		{Releases: []Release{{Name: "zero", ProtocolVersions: []uint16{0}, SchemaVersions: []uint16{1}}}},
		{Releases: []Release{{Name: "empty", ProtocolVersions: []uint16{1}}}},
	}
	for index, matrix := range tests {
		if err := matrix.Validate(); err == nil {
			t.Errorf("test %d Validate() error = nil", index)
		}
	}
}
