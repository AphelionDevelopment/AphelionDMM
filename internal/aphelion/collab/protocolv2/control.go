package protocolv2

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"time"
)

type RoomCreateControl struct{}

type AdmissionControl struct {
	Digest     [32]byte  `json:"digest"`
	Role       Role      `json:"role"`
	ExpiresAt  time.Time `json:"expires_at"`
	BoundActor ActorKey  `json:"bound_actor"`
}

type AdmissionReplaceControl struct {
	Generation uint64             `json:"generation"`
	Admissions []AdmissionControl `json:"admissions"`
}

type OwnerTransferControl struct {
	Transfer OwnershipTransferred `json:"transfer"`
}

type ConnectControl struct {
	Admission [32]byte `json:"admission"`
}

type HeartbeatControl struct{}

type DisconnectControl struct {
	Code string `json:"code"`
}

func SignControl(entropy io.Reader, signer ed25519.PrivateKey, header Header, payload any) (Envelope, error) {
	if header.Route != RouteControl || header.Kind == KindApplication || !header.Kind.valid() {
		return Envelope{}, fmt.Errorf("protocol v2 control kind is invalid")
	}
	if len(signer) != ed25519.PrivateKeySize {
		return Envelope{}, fmt.Errorf("protocol v2 control signer is invalid")
	}
	if subtle.ConstantTimeCompare(header.Sender[:], signer.Public().(ed25519.PublicKey)) != 1 {
		return Envelope{}, ErrSenderKeyMismatch
	}
	expected, err := newControlPayload(header.Kind)
	if err != nil {
		return Envelope{}, err
	}
	if reflect.TypeOf(payload) != reflect.TypeOf(expected).Elem() {
		return Envelope{}, fmt.Errorf("protocol v2 control payload type is %T", payload)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("encode protocol v2 control payload: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > MaxCiphertextBytes {
		return Envelope{}, fmt.Errorf("protocol v2 control payload is outside bounds")
	}
	if entropy == nil {
		entropy = rand.Reader
	}
	var nonce Nonce
	if _, err := io.ReadFull(entropy, nonce[:]); err != nil {
		return Envelope{}, fmt.Errorf("generate protocol v2 control nonce: %w", err)
	}
	aad, err := header.AuthenticatedBytes()
	if err != nil {
		return Envelope{}, err
	}
	var signature Signature
	copy(signature[:], ed25519.Sign(signer, signatureInput(aad, nonce, encoded)))
	return Envelope{Header: header, Nonce: nonce, Ciphertext: encoded, Signature: signature}, nil
}

func VerifyControl(envelope Envelope) (any, error) {
	if envelope.Header.Route != RouteControl || envelope.Header.Kind == KindApplication {
		return nil, fmt.Errorf("protocol v2 envelope is not a control message")
	}
	aad, err := envelope.Header.AuthenticatedBytes()
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(ed25519.PublicKey(envelope.Header.Sender[:]), signatureInput(aad, envelope.Nonce, envelope.Ciphertext), envelope.Signature[:]) {
		return nil, ErrSignatureInvalid
	}
	payload, err := newControlPayload(envelope.Header.Kind)
	if err != nil {
		return nil, err
	}
	if err := decodeApplicationJSON(envelope.Ciphertext, payload); err != nil {
		return nil, fmt.Errorf("decode protocol v2 control payload: %w", err)
	}
	return payload, nil
}

func newControlPayload(kind Kind) (any, error) {
	switch kind {
	case KindRoomCreate:
		return &RoomCreateControl{}, nil
	case KindAdmissionReplace:
		return &AdmissionReplaceControl{}, nil
	case KindOwnerTransfer:
		return &OwnerTransferControl{}, nil
	case KindConnect:
		return &ConnectControl{}, nil
	case KindHeartbeat:
		return &HeartbeatControl{}, nil
	case KindDisconnect:
		return &DisconnectControl{}, nil
	default:
		return nil, fmt.Errorf("protocol v2 control kind %d is unsupported", kind)
	}
}
