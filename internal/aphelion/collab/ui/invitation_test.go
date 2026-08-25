package ui

import (
	"strings"
	"testing"
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

func TestParseInvitationRejectsUnknownOrOversizedContent(t *testing.T) {
	t.Parallel()

	if _, err := ParseInvitation(`{"base_url":"http://127.0.0.1:1","origin":"http://127.0.0.1:1","session_id":"session","token":"secret","extra":true}`); err == nil {
		t.Fatal("invitation with unknown field was accepted")
	}
	if _, err := ParseInvitation(strings.Repeat("x", maxEncodedInvitationBytes+1)); err == nil {
		t.Fatal("oversized invitation was accepted")
	}
}
