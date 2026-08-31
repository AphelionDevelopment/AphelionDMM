package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
)

const canonicalMapDomain = "apheliondmm.map.v1"

func (snapshot Snapshot) Hash() (string, error) {
	if err := snapshot.Validate(); err != nil {
		return "", err
	}

	tiles := append([]Tile(nil), snapshot.Tiles...)
	sort.Slice(tiles, func(left int, right int) bool {
		if tiles[left].Coord.Z != tiles[right].Coord.Z {
			return tiles[left].Coord.Z < tiles[right].Coord.Z
		}
		if tiles[left].Coord.Y != tiles[right].Coord.Y {
			return tiles[left].Coord.Y < tiles[right].Coord.Y
		}
		return tiles[left].Coord.X < tiles[right].Coord.X
	})

	var canonical bytes.Buffer
	writeString(&canonical, canonicalMapDomain)
	writeUint64(&canonical, uint64(snapshot.MaxX))
	writeUint64(&canonical, uint64(snapshot.MaxY))
	writeUint64(&canonical, uint64(snapshot.MaxZ))
	writeUint64(&canonical, uint64(len(tiles)))
	for _, tile := range tiles {
		writeUint64(&canonical, uint64(tile.Coord.X))
		writeUint64(&canonical, uint64(tile.Coord.Y))
		writeUint64(&canonical, uint64(tile.Coord.Z))
		writeUint64(&canonical, uint64(len(tile.State.Prefabs)))
		for _, prefab := range tile.State.Prefabs {
			writeString(&canonical, string(prefab.StableID))
			writeString(&canonical, prefab.Path)

			keys := make([]string, 0, len(prefab.Vars))
			for key := range prefab.Vars {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			writeUint64(&canonical, uint64(len(keys)))
			for _, key := range keys {
				writeString(&canonical, key)
				writeString(&canonical, prefab.Vars[key])
			}
		}
	}

	digest := sha256.Sum256(canonical.Bytes())
	return hex.EncodeToString(digest[:]), nil
}

func ValidateSHA256(name string, value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("%s: length is %d, want %d lowercase hexadecimal characters", name, len(value), sha256.Size*2)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(decoded) != value {
		return fmt.Errorf("%s: must be lowercase hexadecimal SHA-256", name)
	}
	return nil
}

// Validate checks whether a snapshot is safe and canonical enough to hash or apply.
func (snapshot Snapshot) Validate() error {
	if snapshot.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("protocol version is %d, want %d", snapshot.ProtocolVersion, ProtocolVersion)
	}
	if snapshot.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema version is %d, want %d", snapshot.SchemaVersion, SchemaVersion)
	}
	if err := snapshot.DocumentID.Validate(); err != nil {
		return err
	}
	if err := ValidateSHA256("environment hash", snapshot.EnvironmentHash); err != nil {
		return err
	}
	if _, err := snapshot.CellCount(); err != nil {
		return err
	}

	coordinates := make(map[Coord]struct{}, len(snapshot.Tiles))
	stableIDs := make(map[StableID]struct{})
	for _, tile := range snapshot.Tiles {
		if !snapshot.Contains(tile.Coord) {
			return fmt.Errorf("coordinate (%d,%d,%d) is outside dimensions (%d,%d,%d)", tile.Coord.X, tile.Coord.Y, tile.Coord.Z, snapshot.MaxX, snapshot.MaxY, snapshot.MaxZ)
		}
		if _, exists := coordinates[tile.Coord]; exists {
			return fmt.Errorf("duplicate coordinate (%d,%d,%d)", tile.Coord.X, tile.Coord.Y, tile.Coord.Z)
		}
		coordinates[tile.Coord] = struct{}{}

		for _, prefab := range tile.State.Prefabs {
			if err := prefab.StableID.Validate(); err != nil {
				return err
			}
			if _, exists := stableIDs[prefab.StableID]; exists {
				return fmt.Errorf("duplicate stable id %q", prefab.StableID)
			}
			stableIDs[prefab.StableID] = struct{}{}
		}
	}
	return nil
}

func (snapshot Snapshot) Contains(coord Coord) bool {
	return coord.X >= 1 && coord.X <= snapshot.MaxX &&
		coord.Y >= 1 && coord.Y <= snapshot.MaxY &&
		coord.Z >= 1 && coord.Z <= snapshot.MaxZ
}

func writeString(buffer *bytes.Buffer, value string) {
	writeUint64(buffer, uint64(len(value)))
	_, _ = buffer.WriteString(value)
}

func writeUint64(buffer *bytes.Buffer, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = buffer.Write(encoded[:])
}
