package protocolv2

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInvitationRoundTripVerifiesOwnerAndKeepsRelayConfigurable(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0xa1}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	invitation := invitationFixture(publicKey, now)

	encoded, err := EncodeInvitation(invitation, privateKey)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(encoded, "apheliondmm://join?invite="))
	fixture, err := os.ReadFile("testdata/v2/editor_invitation.txt")
	if err != nil {
		t.Fatalf("read invitation fixture: %v; generated=%s", err, encoded)
	}
	require.Equal(t, strings.TrimSpace(string(fixture)), encoded)
	parsed, err := ParseInvitation(encoded, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, invitation.Version, parsed.Version)
	require.Equal(t, "https://mapping.a13.info", parsed.RelayURL)
	require.Equal(t, invitation.RoomID, parsed.RoomID)
	require.Equal(t, invitation.Role, parsed.Role)
	require.Equal(t, invitation.Admission, parsed.Admission)
	require.Equal(t, invitation.GroupKey, parsed.GroupKey)
	require.Equal(t, invitation.OwnerPublicKey, parsed.OwnerPublicKey)
	require.NotEqual(t, Signature{}, parsed.OwnerSignature)
}

func TestInvitationRejectsUnsafeEndpointExpiryAndWrongSigner(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0xb1}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	tests := map[string]func(*Invitation){
		"cleartext remote": func(value *Invitation) { value.RelayURL = "http://relay.example" },
		"userinfo":         func(value *Invitation) { value.RelayURL = "https://user@example.test" },
		"query":            func(value *Invitation) { value.RelayURL = "https://example.test?secret=value" },
		"fragment":         func(value *Invitation) { value.RelayURL = "https://example.test/#fragment" },
		"long lifetime":    func(value *Invitation) { value.ExpiresAt = value.IssuedAt.Add(MaxInvitationLifetime + time.Second) },
		"owner mismatch":   func(value *Invitation) { value.OwnerPublicKey[0] ^= 0xff },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			invitation := invitationFixture(publicKey, now)
			mutate(&invitation)
			_, err := EncodeInvitation(invitation, privateKey)
			require.Error(t, err)
		})
	}

	loopback := invitationFixture(publicKey, now)
	loopback.RelayURL = "http://127.0.0.1:8080"
	encoded, err := EncodeInvitation(loopback, privateKey)
	require.NoError(t, err)
	_, err = ParseInvitation(encoded, loopback.ExpiresAt)
	require.ErrorIs(t, err, ErrInvitationExpired)

	replacement := byte('A')
	if encoded[len(encoded)-1] == replacement {
		replacement = 'B'
	}
	tampered := encoded[:len(encoded)-1] + string(replacement)
	_, err = ParseInvitation(tampered, now)
	require.Error(t, err)

	parsedURI, err := url.Parse(encoded)
	require.NoError(t, err)
	inner, err := base64.RawURLEncoding.DecodeString(parsedURI.Query().Get("invite"))
	require.NoError(t, err)
	query := parsedURI.Query()
	query.Set("invite", base64.RawURLEncoding.EncodeToString(append(inner, []byte(` {}`)...)))
	parsedURI.RawQuery = query.Encode()
	_, err = ParseInvitation(parsedURI.String(), now)
	require.Error(t, err)
}

func invitationFixture(publicKey ed25519.PublicKey, now time.Time) Invitation {
	var owner ActorKey
	copy(owner[:], publicKey)
	return Invitation{
		Version: Version, RelayURL: "https://mapping.a13.info", RoomID: roomIDFixture(0xc1), Role: RoleEditor,
		Admission: capabilityFixture(0xc2), GroupKey: groupKeyFixture(0xc3), OwnerPublicKey: owner,
		DocumentID: "01890f3e-7b5c-7abc-8def-0123456789ab", EnvironmentHash: strings.Repeat("a", 64),
		ManifestSHA256: digestFixture(0xc4), IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}
}

func capabilityFixture(value byte) (capability Capability) {
	for index := range capability {
		capability[index] = value
	}
	return capability
}

func digestFixture(value byte) (digest Digest) {
	for index := range digest {
		digest[index] = value
	}
	return digest
}
