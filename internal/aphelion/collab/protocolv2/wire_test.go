package protocolv2

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvelopeGoldenEncoding(t *testing.T) {
	header := Header{
		Version:   Version,
		Route:     RouteOwner,
		Kind:      KindApplication,
		RoomID:    roomIDFixture(0x01),
		MessageID: messageIDFixture(0x11),
		Sender:    actorKeyFixture(0x21),
		Sequence:  0x0102030405060708,
	}
	envelope := Envelope{
		Header:     header,
		Nonce:      nonceFixture(0x41),
		Ciphertext: []byte{0xaa, 0xbb, 0xcc},
		Signature:  signatureFixture(0x59),
	}

	encoded, err := MarshalEnvelope(envelope)
	require.NoError(t, err)
	require.Equal(t, mustReadHex(t, "testdata/v2/route_owner.hex"), encoded)

	decoded, err := UnmarshalEnvelope(encoded)
	require.NoError(t, err)
	require.Equal(t, envelope, decoded)
}

func TestActorRouteGoldenEncoding(t *testing.T) {
	envelope := Envelope{
		Header: Header{
			Version: Version, Route: RouteActor, Kind: KindApplication,
			RoomID: roomIDFixture(0x01), MessageID: messageIDFixture(0x11), Sender: actorKeyFixture(0x21),
			Recipient: actorKeyFixture(0x61), Sequence: 2,
		},
		Nonce: nonceFixture(0x81), Ciphertext: []byte{1, 2, 3, 4}, Signature: signatureFixture(0xa1),
	}
	encoded, err := MarshalEnvelope(envelope)
	require.NoError(t, err)
	require.Equal(t, mustReadHex(t, "testdata/v2/route_actor.hex"), encoded)
}

func TestEnvelopeRejectsMalformedRoutingAndBounds(t *testing.T) {
	valid := Envelope{
		Header: Header{
			Version: Version, Route: RouteOwner, Kind: KindApplication,
			RoomID: roomIDFixture(1), MessageID: messageIDFixture(2), Sender: actorKeyFixture(3), Sequence: 1,
		},
		Nonce: nonceFixture(4), Ciphertext: []byte{1}, Signature: signatureFixture(5),
	}
	tests := map[string]func(*Envelope){
		"version":          func(value *Envelope) { value.Header.Version++ },
		"route":            func(value *Envelope) { value.Header.Route = 0 },
		"kind":             func(value *Envelope) { value.Header.Kind = 0 },
		"room":             func(value *Envelope) { value.Header.RoomID = RoomID{} },
		"message":          func(value *Envelope) { value.Header.MessageID = MessageID{} },
		"sender":           func(value *Envelope) { value.Header.Sender = ActorKey{} },
		"sequence":         func(value *Envelope) { value.Header.Sequence = 0 },
		"owner recipient":  func(value *Envelope) { value.Header.Recipient = actorKeyFixture(9) },
		"actor recipient":  func(value *Envelope) { value.Header.Route = RouteActor },
		"empty ciphertext": func(value *Envelope) { value.Ciphertext = nil },
		"large ciphertext": func(value *Envelope) { value.Ciphertext = make([]byte, MaxCiphertextBytes+1) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			_, err := MarshalEnvelope(candidate)
			require.Error(t, err)
		})
	}

	encoded, err := MarshalEnvelope(valid)
	require.NoError(t, err)
	_, err = UnmarshalEnvelope(encoded[:len(encoded)-1])
	require.Error(t, err)
	_, err = UnmarshalEnvelope(append(encoded, 0))
	require.Error(t, err)
}

func TestSequenceValidatorRejectsReplayAndRegression(t *testing.T) {
	validator := SequenceValidator{}
	header := Header{Version: Version, Route: RouteOwner, Kind: KindApplication, RoomID: roomIDFixture(1), MessageID: messageIDFixture(2), Sender: actorKeyFixture(3), Sequence: 10}
	require.NoError(t, validator.Accept(header))
	require.ErrorContains(t, validator.Accept(header), "monotonic")
	header.Sequence = 9
	require.ErrorContains(t, validator.Accept(header), "monotonic")
	header.Sequence = 11
	require.NoError(t, validator.Accept(header))
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	require.NoError(t, err)
	return decoded
}

func mustReadHex(t *testing.T, path string) []byte {
	t.Helper()
	encoded, err := os.ReadFile(path)
	require.NoError(t, err)
	return mustDecodeHex(t, strings.TrimSpace(string(encoded)))
}

func roomIDFixture(first byte) (value RoomID) {
	for index := range value {
		value[index] = first + byte(index)
	}
	return value
}

func messageIDFixture(first byte) (value MessageID) {
	for index := range value {
		value[index] = first + byte(index)
	}
	return value
}

func actorKeyFixture(first byte) (value ActorKey) {
	for index := range value {
		value[index] = first + byte(index)
	}
	return value
}

func nonceFixture(first byte) (value Nonce) {
	for index := range value {
		value[index] = first + byte(index)
	}
	return value
}

func signatureFixture(first byte) (value Signature) {
	for index := range value {
		value[index] = first + byte(index)
	}
	return value
}
