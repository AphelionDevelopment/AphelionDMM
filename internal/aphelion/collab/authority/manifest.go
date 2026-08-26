package authority

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"sdmm/internal/aphelion/collab/protocolv2"
)

func DigestManifest(manifest protocolv2.RoleManifest) ([32]byte, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return [32]byte{}, fmt.Errorf("encode manifest: %w", err)
	}
	return sha256.Sum256(encoded), nil
}

const MaxDisplayNameBytes = 96

type SignedManifest struct {
	Manifest  protocolv2.RoleManifest `json:"manifest"`
	Signature protocolv2.Signature    `json:"signature"`
}

func SignManifest(manifest protocolv2.RoleManifest, privateKey ed25519.PrivateKey) (SignedManifest, error) {
	if err := validateManifest(manifest); err != nil {
		return SignedManifest{}, err
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return SignedManifest{}, fmt.Errorf("manifest signing key is invalid")
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	if !equalActorKey(manifest.OwnerKey, publicKey) {
		return SignedManifest{}, fmt.Errorf("manifest owner does not match signing key")
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return SignedManifest{}, fmt.Errorf("encode manifest: %w", err)
	}
	var signature protocolv2.Signature
	copy(signature[:], ed25519.Sign(privateKey, encoded))
	return SignedManifest{Manifest: manifest, Signature: signature}, nil
}

func VerifyManifest(signed SignedManifest) error {
	if err := validateManifest(signed.Manifest); err != nil {
		return err
	}
	encoded, err := json.Marshal(signed.Manifest)
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(signed.Manifest.OwnerKey[:]), encoded, signed.Signature[:]) {
		return fmt.Errorf("manifest signature is invalid")
	}
	return nil
}

func validateManifest(manifest protocolv2.RoleManifest) error {
	if manifest.RoomID == (protocolv2.RoomID{}) || manifest.OwnerKey == (protocolv2.ActorKey{}) {
		return fmt.Errorf("manifest room or owner is empty")
	}
	if manifest.Generation == 0 || manifest.GroupKeyGeneration == 0 {
		return fmt.Errorf("manifest generation is zero")
	}
	if len(manifest.Members) == 0 {
		return fmt.Errorf("manifest has no members")
	}
	seenKeys := make(map[protocolv2.ActorKey]struct{}, len(manifest.Members))
	seenIDs := make(map[string]struct{}, len(manifest.Members))
	ownerCount := 0
	for _, member := range manifest.Members {
		if member.ActorKey == (protocolv2.ActorKey{}) {
			return fmt.Errorf("manifest member key is empty")
		}
		if _, exists := seenKeys[member.ActorKey]; exists {
			return fmt.Errorf("manifest contains duplicate actor key")
		}
		seenKeys[member.ActorKey] = struct{}{}
		if err := member.ActorID.Validate(); err != nil {
			return fmt.Errorf("manifest actor ID is invalid: %w", err)
		}
		if _, exists := seenIDs[string(member.ActorID)]; exists {
			return fmt.Errorf("manifest contains duplicate actor ID")
		}
		seenIDs[string(member.ActorID)] = struct{}{}
		name := strings.TrimSpace(member.DisplayName)
		if name == "" || !utf8.ValidString(name) || len(name) > MaxDisplayNameBytes {
			return fmt.Errorf("manifest display name is invalid")
		}
		switch member.Role {
		case protocolv2.RoleOwner:
			ownerCount++
			if member.ActorKey != manifest.OwnerKey {
				return fmt.Errorf("manifest owner member does not match owner key")
			}
		case protocolv2.RoleEditor, protocolv2.RoleViewer:
		default:
			return fmt.Errorf("manifest member role is invalid")
		}
	}
	if ownerCount != 1 {
		return fmt.Errorf("manifest must contain exactly one owner")
	}
	return nil
}

func equalActorKey(actorKey protocolv2.ActorKey, publicKey ed25519.PublicKey) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	for index := range actorKey {
		if actorKey[index] != publicKey[index] {
			return false
		}
	}
	return true
}
