package protocolv2

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"sdmm/internal/aphelion/collab/model"
)

const MaxApplicationBytes = MaxCiphertextBytes - 16

type ApplicationType string

const (
	ApplicationSyncHello            ApplicationType = "sync_hello"
	ApplicationSyncReplay           ApplicationType = "sync_replay"
	ApplicationSyncSnapshot         ApplicationType = "sync_snapshot"
	ApplicationSyncComplete         ApplicationType = "sync_complete"
	ApplicationOperationSubmit      ApplicationType = "operation_submit"
	ApplicationOperationAccepted    ApplicationType = "operation_accepted"
	ApplicationRevisionAcknowledged ApplicationType = "revision_acknowledged"
	ApplicationOperationRejected    ApplicationType = "operation_rejected"
	ApplicationInverseRequest       ApplicationType = "inverse_request"
	ApplicationProfileUpdate        ApplicationType = "profile_update"
	ApplicationPresenceUpdate       ApplicationType = "presence_update"
	ApplicationRoleManifest         ApplicationType = "role_manifest"
	ApplicationOwnershipOffer       ApplicationType = "ownership_offer"
	ApplicationOwnershipAccept      ApplicationType = "ownership_accept"
	ApplicationOwnershipTransferred ApplicationType = "ownership_transferred"
	ApplicationSessionNotice        ApplicationType = "session_notice"
)

type Role string

const (
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleOwner  Role = "owner"
)

type SyncHello struct {
	ActorKey        ActorKey            `json:"actor_key"`
	ActorID         model.ActorID       `json:"actor_id"`
	DisplayName     string              `json:"display_name"`
	Role            Role                `json:"role"`
	Revision        model.Revision      `json:"revision"`
	MapHash         string              `json:"map_hash"`
	EnvironmentHash string              `json:"environment_hash"`
	Pending         []model.OperationID `json:"pending_operation_ids"`
	Invitation      *Invitation         `json:"invitation,omitempty"`
}

type SyncReplay struct {
	Operations []model.AcceptedOperation `json:"operations"`
	Revision   model.Revision            `json:"revision"`
	MapHash    string                    `json:"map_hash"`
}

type SyncSnapshot struct {
	Snapshot model.Snapshot `json:"snapshot"`
	MapHash  string         `json:"map_hash"`
}

type SyncComplete struct {
	Revision model.Revision `json:"revision"`
	MapHash  string         `json:"map_hash"`
}

type OperationSubmit struct {
	Operation model.Operation `json:"operation"`
}

type OperationAccepted struct {
	Operation model.AcceptedOperation `json:"operation"`
	MapHash   string                  `json:"map_hash"`
}

type RevisionAcknowledged struct {
	Revision model.Revision `json:"revision"`
	MapHash  string         `json:"map_hash"`
}

type OperationRejected struct {
	OperationID         model.OperationID `json:"operation_id"`
	Code                string            `json:"code"`
	Message             string            `json:"message"`
	Revision            model.Revision    `json:"revision"`
	MapHash             string            `json:"map_hash"`
	AuthoritativeValues []model.Tile      `json:"authoritative_values,omitempty"`
}

type InverseRequest struct {
	OperationID model.OperationID `json:"operation_id"`
	InverseID   model.OperationID `json:"inverse_id"`
}

type ProfileUpdate struct {
	DisplayName string `json:"display_name"`
	Sequence    uint64 `json:"sequence"`
}

type PresenceSelection struct {
	Min model.Coord `json:"min"`
	Max model.Coord `json:"max"`
}

type PresenceUpdate struct {
	Sequence  uint64             `json:"sequence"`
	Cursor    *model.Coord       `json:"cursor,omitempty"`
	Selection *PresenceSelection `json:"selection,omitempty"`
	Status    string             `json:"status"`
}

type Member struct {
	ActorKey    ActorKey      `json:"actor_key"`
	ActorID     model.ActorID `json:"actor_id"`
	Role        Role          `json:"role"`
	DisplayName string        `json:"display_name"`
}

type RoleManifest struct {
	RoomID             RoomID   `json:"room_id"`
	Generation         uint64   `json:"generation"`
	OwnerKey           ActorKey `json:"owner_key"`
	GroupKeyGeneration uint64   `json:"group_key_generation"`
	Members            []Member `json:"members"`
}

type OwnershipOffer struct {
	TargetActor ActorKey     `json:"target_actor"`
	NewOwnerKey ActorKey     `json:"new_owner_key"`
	Generation  uint64       `json:"generation"`
	Manifest    RoleManifest `json:"manifest"`
}

type OwnershipAccept struct {
	OfferSHA256          [32]byte  `json:"offer_sha256"`
	NewOwnerKeySignature Signature `json:"new_owner_key_signature"`
}

type OwnershipTransferred struct {
	Manifest          RoleManifest `json:"manifest"`
	OfferSHA256       [32]byte     `json:"offer_sha256"`
	OldOwnerSignature Signature    `json:"old_owner_signature"`
	NewOwnerSignature Signature    `json:"new_owner_signature"`
}

func DigestOwnershipOffer(offer OwnershipOffer) ([32]byte, error) {
	encoded, err := json.Marshal(offer)
	if err != nil {
		return [32]byte{}, fmt.Errorf("encode ownership offer: %w", err)
	}
	return sha256.Sum256(encoded), nil
}

type SessionNotice struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DecodedApplication struct {
	Type    ApplicationType
	Payload any
}

type applicationEnvelope struct {
	Type    ApplicationType `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func EncodeApplication(messageType ApplicationType, payload any) ([]byte, error) {
	expected, err := newApplicationPayload(messageType)
	if err != nil {
		return nil, err
	}
	want := reflect.TypeOf(expected).Elem()
	actual := reflect.TypeOf(payload)
	if actual == nil || (actual != want && actual != reflect.PointerTo(want)) {
		return nil, fmt.Errorf("protocol v2 %s payload type is %T, want %s", messageType, payload, want)
	}
	payloadData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode protocol v2 %s payload: %w", messageType, err)
	}
	encoded, err := json.Marshal(applicationEnvelope{Type: messageType, Payload: payloadData})
	if err != nil {
		return nil, fmt.Errorf("encode protocol v2 application envelope: %w", err)
	}
	if len(encoded) > MaxApplicationBytes {
		return nil, fmt.Errorf("protocol v2 application message is %d bytes, maximum is %d", len(encoded), MaxApplicationBytes)
	}
	return encoded, nil
}

func DecodeApplication(encoded []byte) (DecodedApplication, error) {
	if len(encoded) == 0 || len(encoded) > MaxApplicationBytes {
		return DecodedApplication{}, fmt.Errorf("protocol v2 application message is %d bytes, maximum is %d", len(encoded), MaxApplicationBytes)
	}
	var envelope applicationEnvelope
	if err := decodeApplicationJSON(encoded, &envelope); err != nil {
		return DecodedApplication{}, fmt.Errorf("decode protocol v2 application envelope: %w", err)
	}
	payload, err := newApplicationPayload(envelope.Type)
	if err != nil {
		return DecodedApplication{}, err
	}
	if len(envelope.Payload) == 0 || string(envelope.Payload) == "null" {
		return DecodedApplication{}, fmt.Errorf("protocol v2 %s payload is empty", envelope.Type)
	}
	if err := decodeApplicationJSON(envelope.Payload, payload); err != nil {
		return DecodedApplication{}, fmt.Errorf("decode protocol v2 %s payload: %w", envelope.Type, err)
	}
	return DecodedApplication{Type: envelope.Type, Payload: payload}, nil
}

func decodeApplicationJSON(encoded []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("JSON contains trailing data")
	}
	return nil
}

func newApplicationPayload(messageType ApplicationType) (any, error) {
	switch messageType {
	case ApplicationSyncHello:
		return &SyncHello{}, nil
	case ApplicationSyncReplay:
		return &SyncReplay{}, nil
	case ApplicationSyncSnapshot:
		return &SyncSnapshot{}, nil
	case ApplicationSyncComplete:
		return &SyncComplete{}, nil
	case ApplicationOperationSubmit:
		return &OperationSubmit{}, nil
	case ApplicationOperationAccepted:
		return &OperationAccepted{}, nil
	case ApplicationRevisionAcknowledged:
		return &RevisionAcknowledged{}, nil
	case ApplicationOperationRejected:
		return &OperationRejected{}, nil
	case ApplicationInverseRequest:
		return &InverseRequest{}, nil
	case ApplicationProfileUpdate:
		return &ProfileUpdate{}, nil
	case ApplicationPresenceUpdate:
		return &PresenceUpdate{}, nil
	case ApplicationRoleManifest:
		return &RoleManifest{}, nil
	case ApplicationOwnershipOffer:
		return &OwnershipOffer{}, nil
	case ApplicationOwnershipAccept:
		return &OwnershipAccept{}, nil
	case ApplicationOwnershipTransferred:
		return &OwnershipTransferred{}, nil
	case ApplicationSessionNotice:
		return &SessionNotice{}, nil
	default:
		return nil, fmt.Errorf("protocol v2 application type %q is unsupported", messageType)
	}
}
