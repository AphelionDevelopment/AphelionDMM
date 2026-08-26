package authority

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
)

func TestManifestSigningAndVerification(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := testManifest(t, publicKey)

	signed, err := SignManifest(manifest, privateKey)
	require.NoError(t, err)
	require.NoError(t, VerifyManifest(signed))
	signed.Manifest.Members[0].DisplayName = "Tampered"
	require.ErrorContains(t, VerifyManifest(signed), "signature")
}

func TestManifestRejectsDuplicateMembersAndWrongOwner(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := testManifest(t, publicKey)
	manifest.Members = append(manifest.Members, manifest.Members[0])

	_, err = SignManifest(manifest, privateKey)
	require.ErrorContains(t, err, "duplicate")
	manifest = testManifest(t, publicKey)
	manifest.OwnerKey[0] ^= 0xff
	_, err = SignManifest(manifest, privateKey)
	require.ErrorContains(t, err, "owner")
}

func testManifest(t *testing.T, owner ed25519.PublicKey) protocolv2.RoleManifest {
	t.Helper()
	actorID, err := model.NewActorID()
	require.NoError(t, err)
	var ownerKey protocolv2.ActorKey
	copy(ownerKey[:], owner)
	return protocolv2.RoleManifest{
		RoomID:             protocolv2.RoomID{1},
		Generation:         1,
		OwnerKey:           ownerKey,
		GroupKeyGeneration: 1,
		Members: []protocolv2.Member{{
			ActorKey:    ownerKey,
			ActorID:     actorID,
			Role:        protocolv2.RoleOwner,
			DisplayName: "Test Owner",
		}},
	}
}
