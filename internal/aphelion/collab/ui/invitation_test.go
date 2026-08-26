package ui

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/protocolv2"
)

func TestEncodeParseInvitationRoundTripWithoutUsingString(t *testing.T) {
	t.Parallel()

	want := Invitation{
		BaseURL:   "http://127.0.0.1:12345",
		Origin:    "http://127.0.0.1:12345",
		SessionID: "session-1",
		Token:     "join-secret",
	}
	encoded, err := EncodeInvitation(want)
	if err != nil {
		t.Fatal(err)
	}
	if encoded == want.String() || !strings.Contains(encoded, "join-secret") {
		t.Fatalf("encoded invitation = %q", encoded)
	}
	parsed, err := ParseInvitation(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != want {
		t.Fatalf("parsed invitation = %#v, want %#v", parsed, want)
	}
}

func TestParseInvitationAcceptsSignedProtocolV2URI(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x21}, ed25519.SeedSize))
	var owner protocolv2.ActorKey
	copy(owner[:], privateKey.Public().(ed25519.PublicKey))
	v2 := protocolv2.Invitation{Version: 2, RelayURL: "https://mapping.a13.info", RoomID: protocolv2.RoomID{1}, Role: protocolv2.RoleEditor, Admission: protocolv2.Capability{2}, GroupKey: protocolv2.GroupKey{3}, OwnerPublicKey: owner, DocumentID: "01890f3e-7b5c-7abc-8def-0123456789ab", EnvironmentHash: strings.Repeat("a", 64), ManifestSHA256: protocolv2.Digest{4}, IssuedAt: time.Now().Add(-time.Minute).UTC(), ExpiresAt: time.Now().Add(time.Hour).UTC()}
	encoded, err := protocolv2.EncodeInvitation(v2, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseInvitation(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ProtocolV2 == nil || parsed.ProtocolV2.RoomID != v2.RoomID || parsed.ProtocolV2.Role != v2.Role {
		t.Fatalf("parsed protocol-v2 invitation = %#v", parsed.ProtocolV2)
	}
	roundTrip, err := EncodeInvitation(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip != encoded {
		t.Fatalf("protocol-v2 round trip changed invitation")
	}
}

func TestParseInvitationRejectsUnknownOrOversizedContent(t *testing.T) {
	t.Parallel()

	if _, err := ParseInvitation(`{"base_url":"http://127.0.0.1:1","origin":"http://127.0.0.1:1","session_id":"session","token":"secret","extra":true}`); err == nil {
		t.Fatal("invitation with unknown field was accepted")
	}
	if _, err := ParseInvitation(strings.Repeat("x", maxEncodedInvitationBytes+1)); err == nil {
		t.Fatal("oversized invitation was accepted")
	}
}

func TestHostedInvitationRequiresSecureOrigin(t *testing.T) {
	t.Parallel()
	want := Invitation{BaseURL: "https://maps.example.test", Origin: "https://maps.example.test", SessionID: "session-1", Token: "invite-secret", TokenExpiresAt: time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC), Hosted: true}
	encoded, err := EncodeInvitation(want)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encoded, `"format_version":1`) {
		t.Fatalf("hosted invitation is not versioned: %s", encoded)
	}
	parsed, err := ParseInvitation(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != want {
		t.Fatalf("parsed hosted invitation = %#v, want %#v", parsed, want)
	}
	want.BaseURL = "http://maps.example.test"
	want.Origin = want.BaseURL
	if _, err := EncodeInvitation(want); err == nil {
		t.Fatal("hosted invitation accepted a cleartext non-loopback origin")
	}
	if _, err := ParseInvitation(`{"base_url":"https://maps.example.test","origin":"https://maps.example.test","session_id":"session-1","token":"invite-secret","hosted":true}`); err == nil {
		t.Fatal("unversioned hosted invitation was accepted")
	}
}

func TestParseInvitationRejectsExpiredOrMissingHostedExpiry(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]string{
		"expired": `{"format_version":1,"base_url":"https://maps.example.test","origin":"https://maps.example.test","session_id":"session-1","token":"invite-secret","expires_at":"2000-01-01T00:00:00Z","hosted":true}`,
		"missing": `{"format_version":1,"base_url":"https://maps.example.test","origin":"https://maps.example.test","session_id":"session-1","token":"invite-secret","hosted":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseInvitation(value); err == nil {
				t.Fatal("invalid hosted invitation expiry was accepted")
			}
		})
	}
}
