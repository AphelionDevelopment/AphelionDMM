package protocolv2

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignedControlRoundTripAndTamperRejection(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x21}, ed25519.SeedSize))
	header := controlHeader(privateKey, KindConnect)
	payload := ConnectControl{Admission: [32]byte{1}}
	envelope, err := SignControl(bytes.NewReader(bytes.Repeat([]byte{0x31}, 24)), privateKey, header, payload)
	require.NoError(t, err)

	decoded, err := VerifyControl(envelope)
	require.NoError(t, err)
	require.Equal(t, &payload, decoded)
	envelope.Ciphertext[0] ^= 0xff
	_, err = VerifyControl(envelope)
	require.ErrorIs(t, err, ErrSignatureInvalid)
}

func TestControlRejectsWrongPayloadAndApplicationKind(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x41}, ed25519.SeedSize))
	_, err := SignControl(nil, privateKey, controlHeader(privateKey, KindConnect), HeartbeatControl{})
	require.ErrorContains(t, err, "payload type")
	_, err = SignControl(nil, privateKey, controlHeader(privateKey, KindApplication), HeartbeatControl{})
	require.ErrorContains(t, err, "control kind")
}

func controlHeader(privateKey ed25519.PrivateKey, kind Kind) Header {
	var sender ActorKey
	copy(sender[:], privateKey.Public().(ed25519.PublicKey))
	return Header{Version: Version, Route: RouteControl, Kind: kind, RoomID: RoomID{1}, MessageID: MessageID{2}, Sender: sender, Sequence: 1}
}
