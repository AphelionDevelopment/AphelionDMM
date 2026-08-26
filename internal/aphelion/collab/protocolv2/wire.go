package protocolv2

import (
	"encoding/binary"
	"fmt"
	"sync"
)

const (
	Version            uint16 = 2
	MaxCiphertextBytes        = 1 << 20

	wireMagicSize       = 4
	wireHeaderSize      = 108
	wireNonceSize       = 24
	wireSignatureSize   = 64
	wireLengthFieldSize = 4
)

var wireMagic = [wireMagicSize]byte{'A', 'D', 'M', 'V'}

type Route uint8

const (
	RouteControl Route = iota + 1
	RouteOwner
	RouteActor
	RouteRoom
	RoutePresence
)

type Kind uint8

const (
	KindRoomCreate Kind = iota + 1
	KindAdmissionReplace
	KindOwnerTransfer
	KindConnect
	KindHeartbeat
	KindDisconnect
	KindApplication Kind = 0x20
)

type RoomID [16]byte
type MessageID [16]byte
type ActorKey [32]byte
type Nonce [wireNonceSize]byte
type Signature [wireSignatureSize]byte

type Header struct {
	Version   uint16
	Route     Route
	Kind      Kind
	RoomID    RoomID
	MessageID MessageID
	Sender    ActorKey
	Recipient ActorKey
	Sequence  uint64
}

type Envelope struct {
	Header     Header
	Nonce      Nonce
	Ciphertext []byte
	Signature  Signature
}

type SequenceValidator struct {
	mutex sync.Mutex
	last  uint64
}

func (validator *SequenceValidator) Accept(header Header) error {
	if err := header.validate(); err != nil {
		return err
	}
	validator.mutex.Lock()
	defer validator.mutex.Unlock()
	if header.Sequence <= validator.last {
		return fmt.Errorf("protocol v2 sequence is not monotonic")
	}
	validator.last = header.Sequence
	return nil
}

func (header Header) AuthenticatedBytes() ([]byte, error) {
	if err := header.validate(); err != nil {
		return nil, err
	}
	encoded := make([]byte, wireHeaderSize)
	binary.BigEndian.PutUint16(encoded[0:2], header.Version)
	encoded[2] = byte(header.Route)
	encoded[3] = byte(header.Kind)
	copy(encoded[4:20], header.RoomID[:])
	copy(encoded[20:36], header.MessageID[:])
	copy(encoded[36:68], header.Sender[:])
	copy(encoded[68:100], header.Recipient[:])
	binary.BigEndian.PutUint64(encoded[100:108], header.Sequence)
	return encoded, nil
}

func MarshalEnvelope(envelope Envelope) ([]byte, error) {
	header, err := envelope.Header.AuthenticatedBytes()
	if err != nil {
		return nil, err
	}
	if len(envelope.Ciphertext) == 0 || len(envelope.Ciphertext) > MaxCiphertextBytes {
		return nil, fmt.Errorf("protocol v2 ciphertext length %d is outside 1..%d", len(envelope.Ciphertext), MaxCiphertextBytes)
	}
	encoded := make([]byte, 0, wireMagicSize+wireHeaderSize+wireNonceSize+wireLengthFieldSize+len(envelope.Ciphertext)+wireSignatureSize)
	encoded = append(encoded, wireMagic[:]...)
	encoded = append(encoded, header...)
	encoded = append(encoded, envelope.Nonce[:]...)
	length := make([]byte, wireLengthFieldSize)
	binary.BigEndian.PutUint32(length, uint32(len(envelope.Ciphertext)))
	encoded = append(encoded, length...)
	encoded = append(encoded, envelope.Ciphertext...)
	encoded = append(encoded, envelope.Signature[:]...)
	return encoded, nil
}

func UnmarshalEnvelope(encoded []byte) (Envelope, error) {
	minimum := wireMagicSize + wireHeaderSize + wireNonceSize + wireLengthFieldSize + 1 + wireSignatureSize
	if len(encoded) < minimum {
		return Envelope{}, fmt.Errorf("protocol v2 envelope is truncated")
	}
	if string(encoded[:wireMagicSize]) != string(wireMagic[:]) {
		return Envelope{}, fmt.Errorf("protocol v2 envelope magic is invalid")
	}
	offset := wireMagicSize
	headerBytes := encoded[offset : offset+wireHeaderSize]
	offset += wireHeaderSize
	header := Header{
		Version:  binary.BigEndian.Uint16(headerBytes[0:2]),
		Route:    Route(headerBytes[2]),
		Kind:     Kind(headerBytes[3]),
		Sequence: binary.BigEndian.Uint64(headerBytes[100:108]),
	}
	copy(header.RoomID[:], headerBytes[4:20])
	copy(header.MessageID[:], headerBytes[20:36])
	copy(header.Sender[:], headerBytes[36:68])
	copy(header.Recipient[:], headerBytes[68:100])
	if err := header.validate(); err != nil {
		return Envelope{}, err
	}
	var nonce Nonce
	copy(nonce[:], encoded[offset:offset+wireNonceSize])
	offset += wireNonceSize
	ciphertextLength := int(binary.BigEndian.Uint32(encoded[offset : offset+wireLengthFieldSize]))
	offset += wireLengthFieldSize
	if ciphertextLength < 1 || ciphertextLength > MaxCiphertextBytes {
		return Envelope{}, fmt.Errorf("protocol v2 ciphertext length %d is outside 1..%d", ciphertextLength, MaxCiphertextBytes)
	}
	wantLength := offset + ciphertextLength + wireSignatureSize
	if len(encoded) != wantLength {
		return Envelope{}, fmt.Errorf("protocol v2 envelope length %d does not match declared length %d", len(encoded), wantLength)
	}
	ciphertext := append([]byte(nil), encoded[offset:offset+ciphertextLength]...)
	offset += ciphertextLength
	var signature Signature
	copy(signature[:], encoded[offset:])
	return Envelope{Header: header, Nonce: nonce, Ciphertext: ciphertext, Signature: signature}, nil
}

func (header Header) validate() error {
	if header.Version != Version {
		return fmt.Errorf("protocol v2 version %d is unsupported", header.Version)
	}
	if header.Route < RouteControl || header.Route > RoutePresence {
		return fmt.Errorf("protocol v2 route %d is invalid", header.Route)
	}
	if !header.Kind.valid() {
		return fmt.Errorf("protocol v2 kind %d is invalid", header.Kind)
	}
	if isZero(header.RoomID[:]) || isZero(header.MessageID[:]) || isZero(header.Sender[:]) {
		return fmt.Errorf("protocol v2 routing identity is empty")
	}
	if header.Sequence == 0 {
		return fmt.Errorf("protocol v2 sequence is zero")
	}
	recipientEmpty := isZero(header.Recipient[:])
	if header.Route == RouteActor && recipientEmpty {
		return fmt.Errorf("protocol v2 actor route recipient is empty")
	}
	if header.Route != RouteActor && !recipientEmpty {
		return fmt.Errorf("protocol v2 recipient is only valid for actor routes")
	}
	return nil
}

func (kind Kind) valid() bool {
	return kind >= KindRoomCreate && kind <= KindDisconnect || kind == KindApplication
}

func isZero(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}
