package protocolv2

import (
	"bytes"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"sdmm/internal/aphelion/collab/model"
)

const (
	MaxInvitationLifetime = 24 * time.Hour
	maxInvitationBytes    = 4096
)

var ErrInvitationExpired = errors.New("protocol v2 invitation is expired")

type Capability [32]byte
type Digest [32]byte

type Invitation struct {
	Version         uint16           `json:"version"`
	RelayURL        string           `json:"relay_url"`
	RoomID          RoomID           `json:"room_id"`
	Role            Role             `json:"role"`
	Admission       Capability       `json:"admission"`
	GroupKey        GroupKey         `json:"group_key"`
	OwnerPublicKey  ActorKey         `json:"owner_public_key"`
	DocumentID      model.DocumentID `json:"document_id"`
	EnvironmentHash string           `json:"environment_hash"`
	ManifestSHA256  Digest           `json:"manifest_sha256"`
	IssuedAt        time.Time        `json:"issued_at"`
	ExpiresAt       time.Time        `json:"expires_at"`
	OwnerSignature  Signature        `json:"owner_signature"`
}

type unsignedInvitation struct {
	Version         uint16           `json:"version"`
	RelayURL        string           `json:"relay_url"`
	RoomID          RoomID           `json:"room_id"`
	Role            Role             `json:"role"`
	Admission       Capability       `json:"admission"`
	GroupKey        GroupKey         `json:"group_key"`
	OwnerPublicKey  ActorKey         `json:"owner_public_key"`
	DocumentID      model.DocumentID `json:"document_id"`
	EnvironmentHash string           `json:"environment_hash"`
	ManifestSHA256  Digest           `json:"manifest_sha256"`
	IssuedAt        time.Time        `json:"issued_at"`
	ExpiresAt       time.Time        `json:"expires_at"`
}

func EncodeInvitation(invitation Invitation, signer ed25519.PrivateKey) (string, error) {
	if len(signer) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("protocol v2 invitation signer private key length is invalid")
	}
	invitation.IssuedAt = invitation.IssuedAt.UTC()
	invitation.ExpiresAt = invitation.ExpiresAt.UTC()
	if err := invitation.validate(); err != nil {
		return "", err
	}
	publicKey := signer.Public().(ed25519.PublicKey)
	if subtle.ConstantTimeCompare(invitation.OwnerPublicKey[:], publicKey) != 1 {
		return "", ErrSenderKeyMismatch
	}
	unsigned := invitation.unsigned()
	signedBytes, err := json.Marshal(unsigned)
	if err != nil {
		return "", fmt.Errorf("encode protocol v2 invitation signature input: %w", err)
	}
	copy(invitation.OwnerSignature[:], ed25519.Sign(signer, signedBytes))
	encoded, err := json.Marshal(invitation)
	if err != nil {
		return "", fmt.Errorf("encode protocol v2 invitation: %w", err)
	}
	if len(encoded) > maxInvitationBytes {
		return "", fmt.Errorf("protocol v2 invitation is too large")
	}
	query := url.Values{}
	query.Set("invite", base64.RawURLEncoding.EncodeToString(encoded))
	return (&url.URL{Scheme: "apheliondmm", Host: "join", RawQuery: query.Encode()}).String(), nil
}

func ParseInvitation(encoded string, now time.Time) (Invitation, error) {
	if len(encoded) == 0 || len(encoded) > maxInvitationBytes*2 {
		return Invitation{}, fmt.Errorf("protocol v2 invitation URI length is invalid")
	}
	parsed, err := url.Parse(encoded)
	if err != nil || parsed.Scheme != "apheliondmm" || parsed.Host != "join" || parsed.Path != "" || parsed.User != nil || parsed.Fragment != "" {
		return Invitation{}, fmt.Errorf("protocol v2 invitation URI is invalid")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) != 1 || len(query["invite"]) != 1 || query.Get("invite") == "" {
		return Invitation{}, fmt.Errorf("protocol v2 invitation URI query is invalid")
	}
	data, err := base64.RawURLEncoding.DecodeString(query.Get("invite"))
	if err != nil || len(data) == 0 || len(data) > maxInvitationBytes {
		return Invitation{}, fmt.Errorf("protocol v2 invitation encoding is invalid")
	}
	var invitation Invitation
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&invitation); err != nil {
		return Invitation{}, fmt.Errorf("decode protocol v2 invitation: %w", err)
	}
	if decoder.More() {
		return Invitation{}, fmt.Errorf("protocol v2 invitation contains trailing data")
	}
	if err := VerifyInvitation(invitation, now); err != nil {
		return Invitation{}, err
	}
	return invitation, nil
}

func VerifyInvitation(invitation Invitation, now time.Time) error {
	if err := invitation.validate(); err != nil {
		return err
	}
	signedBytes, err := json.Marshal(invitation.unsigned())
	if err != nil {
		return fmt.Errorf("encode protocol v2 invitation verification input: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(invitation.OwnerPublicKey[:]), signedBytes, invitation.OwnerSignature[:]) {
		return ErrSignatureInvalid
	}
	if !now.Before(invitation.ExpiresAt) {
		return ErrInvitationExpired
	}
	return nil
}

func (invitation Invitation) unsigned() unsignedInvitation {
	return unsignedInvitation{
		Version: invitation.Version, RelayURL: invitation.RelayURL, RoomID: invitation.RoomID, Role: invitation.Role,
		Admission: invitation.Admission, GroupKey: invitation.GroupKey, OwnerPublicKey: invitation.OwnerPublicKey,
		DocumentID: invitation.DocumentID, EnvironmentHash: invitation.EnvironmentHash,
		ManifestSHA256: invitation.ManifestSHA256, IssuedAt: invitation.IssuedAt, ExpiresAt: invitation.ExpiresAt,
	}
}

func (invitation Invitation) validate() error {
	if invitation.Version != Version {
		return fmt.Errorf("protocol v2 invitation version %d is unsupported", invitation.Version)
	}
	if invitation.Role != RoleViewer && invitation.Role != RoleEditor {
		return fmt.Errorf("protocol v2 invitation role %q is invalid", invitation.Role)
	}
	if isZero(invitation.RoomID[:]) || isZero(invitation.Admission[:]) || isZero(invitation.GroupKey[:]) || isZero(invitation.OwnerPublicKey[:]) || isZero(invitation.ManifestSHA256[:]) {
		return fmt.Errorf("protocol v2 invitation contains an empty identity or secret")
	}
	if err := invitation.DocumentID.Validate(); err != nil {
		return fmt.Errorf("protocol v2 invitation document ID is invalid: %w", err)
	}
	if err := model.ValidateSHA256("protocol v2 invitation environment hash", invitation.EnvironmentHash); err != nil {
		return err
	}
	if invitation.IssuedAt.IsZero() || invitation.ExpiresAt.IsZero() || !invitation.IssuedAt.Before(invitation.ExpiresAt) || invitation.ExpiresAt.Sub(invitation.IssuedAt) > MaxInvitationLifetime {
		return fmt.Errorf("protocol v2 invitation lifetime is invalid")
	}
	return validateRelayURL(invitation.RelayURL)
}

func validateRelayURL(value string) error {
	endpoint, err := url.Parse(value)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
		return fmt.Errorf("protocol v2 relay URL is invalid")
	}
	if endpoint.Scheme == "https" {
		return nil
	}
	hostname := endpoint.Hostname()
	ip := net.ParseIP(hostname)
	if endpoint.Scheme == "http" && (strings.EqualFold(hostname, "localhost") || ip != nil && ip.IsLoopback()) {
		return nil
	}
	return fmt.Errorf("protocol v2 relay URL requires HTTPS outside loopback")
}
