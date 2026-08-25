package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

const maxEncodedInvitationBytes = 4096

type InvitationRole string

const (
	InvitationRoleEditor InvitationRole = "editor"
	InvitationRoleViewer InvitationRole = "viewer"
)

type encodedInvitation struct {
	BaseURL   string    `json:"base_url"`
	Origin    string    `json:"origin"`
	SessionID string    `json:"session_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// EncodeInvitation explicitly serializes the short-lived credential for user-directed transfer.
func EncodeInvitation(invitation Invitation) (string, error) {
	if err := invitation.validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(encodedInvitation{
		BaseURL: invitation.BaseURL, Origin: invitation.Origin, SessionID: invitation.SessionID, Token: invitation.Token, ExpiresAt: invitation.TokenExpiresAt,
	})
	if err != nil {
		return "", fmt.Errorf("encode collaboration invitation: %w", err)
	}
	if len(data) > maxEncodedInvitationBytes {
		return "", fmt.Errorf("collaboration invitation exceeds %d bytes", maxEncodedInvitationBytes)
	}
	return string(data), nil
}

// ParseInvitation decodes a bounded credential pasted by the user without logging its contents.
func ParseInvitation(value string) (Invitation, error) {
	if len(value) == 0 || len(value) > maxEncodedInvitationBytes {
		return Invitation{}, fmt.Errorf("collaboration invitation size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	decoder.DisallowUnknownFields()
	var encoded encodedInvitation
	if err := decoder.Decode(&encoded); err != nil {
		return Invitation{}, fmt.Errorf("decode collaboration invitation: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return Invitation{}, err
	}
	invitation := Invitation{
		BaseURL: encoded.BaseURL, Origin: encoded.Origin, SessionID: encoded.SessionID, Token: encoded.Token, TokenExpiresAt: encoded.ExpiresAt,
	}
	if err := invitation.validate(); err != nil {
		return Invitation{}, err
	}
	return invitation, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("collaboration invitation contains trailing content")
		}
		return fmt.Errorf("decode collaboration invitation trailing content: %w", err)
	}
	return nil
}
