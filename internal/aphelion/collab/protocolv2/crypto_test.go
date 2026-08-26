package protocolv2

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/model"
)

func TestSealAndOpenAuthenticatesHeaderCiphertextAndSigner(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x11}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	header := cryptoHeader(publicKey)
	groupKey := groupKeyFixture(0x22)

	envelope, err := Seal(bytes.NewReader(bytes.Repeat([]byte{0x33}, len(Nonce{}))), groupKey, privateKey, header, []byte("map operation"))
	require.NoError(t, err)
	require.Equal(t, repeatedNonceFixture(0x33), envelope.Nonce)
	plaintext, err := Open(groupKey, publicKey, envelope)
	require.NoError(t, err)
	require.Equal(t, []byte("map operation"), plaintext)

	tampered := envelope
	tampered.Header.Sequence++
	_, err = Open(groupKey, publicKey, tampered)
	require.ErrorIs(t, err, ErrSignatureInvalid)

	tampered = envelope
	tampered.Ciphertext = append([]byte(nil), envelope.Ciphertext...)
	tampered.Ciphertext[0] ^= 0xff
	_, err = Open(groupKey, publicKey, tampered)
	require.ErrorIs(t, err, ErrSignatureInvalid)

	tampered = envelope
	tampered.Signature[0] ^= 0xff
	wrongGroupKey := groupKeyFixture(0x44)
	_, err = Open(wrongGroupKey, publicKey, tampered)
	require.ErrorIs(t, err, ErrSignatureInvalid)

	wrongPrivateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x55}, ed25519.SeedSize))
	_, err = Seal(bytes.NewReader(bytes.Repeat([]byte{0x66}, len(Nonce{}))), groupKey, wrongPrivateKey, header, []byte("map operation"))
	require.ErrorIs(t, err, ErrSenderKeyMismatch)
}

func TestSealUsesFreshNonce(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x77}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	header := cryptoHeader(publicKey)
	first, err := Seal(nil, groupKeyFixture(0x88), privateKey, header, []byte("one"))
	require.NoError(t, err)
	header.MessageID[0]++
	header.Sequence++
	second, err := Seal(nil, groupKeyFixture(0x88), privateKey, header, []byte("two"))
	require.NoError(t, err)
	require.NotEqual(t, first.Nonce, second.Nonce)
}

func TestOpenRejectsWrongGroupKeyAfterValidSignature(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x91}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	envelope, err := Seal(bytes.NewReader(bytes.Repeat([]byte{0x92}, len(Nonce{}))), groupKeyFixture(0x93), privateKey, cryptoHeader(publicKey), []byte("secret"))
	require.NoError(t, err)
	_, err = Open(groupKeyFixture(0x94), publicKey, envelope)
	require.True(t, errors.Is(err, ErrDecryptionFailed))
}

func TestOperationAcceptedEncryptedGoldenFixture(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0xd1}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	header := cryptoHeader(publicKey)
	header.Route = RouteRoom
	header.RoomID = roomIDFixture(0xd2)
	header.MessageID = messageIDFixture(0xd3)
	header.Sequence = 9
	payload := OperationAccepted{
		Operation: model.AcceptedOperation{
			Operation: model.Operation{
				ProtocolVersion: model.ProtocolVersion,
				DocumentID:      "018f0000-0000-7000-8000-000000000001",
				ActorID:         "018f0000-0000-7000-8000-000000000002",
				OperationID:     "018f0000-0000-7000-8000-000000000003",
				EnvironmentHash: strings.Repeat("b", 64),
				BaseMapHash:     strings.Repeat("c", 64),
				Kind:            model.OperationKindTileChange,
				Changes:         []model.TileChange{},
			},
			Revision:   1,
			AcceptedAt: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
		},
		MapHash: strings.Repeat("d", 64),
	}
	plaintext, err := EncodeApplication(ApplicationOperationAccepted, payload)
	require.NoError(t, err)
	envelope, err := Seal(bytes.NewReader(bytes.Repeat([]byte{0xd5}, len(Nonce{}))), groupKeyFixture(0xd4), privateKey, header, plaintext)
	require.NoError(t, err)
	wire, err := MarshalEnvelope(envelope)
	require.NoError(t, err)
	fixture, err := os.ReadFile("testdata/v2/operation_accepted.hex")
	if err != nil {
		t.Fatalf("read operation fixture: %v; generated=%s", err, fmt.Sprintf("%x", wire))
	}
	require.Equal(t, mustDecodeHex(t, strings.TrimSpace(string(fixture))), wire)

	decodedEnvelope, err := UnmarshalEnvelope(wire)
	require.NoError(t, err)
	decodedPlaintext, err := Open(groupKeyFixture(0xd4), publicKey, decodedEnvelope)
	require.NoError(t, err)
	decodedApplication, err := DecodeApplication(decodedPlaintext)
	require.NoError(t, err)
	require.Equal(t, &payload, decodedApplication.Payload)
}

func cryptoHeader(publicKey ed25519.PublicKey) Header {
	var sender ActorKey
	copy(sender[:], publicKey)
	return Header{Version: Version, Route: RouteOwner, Kind: KindApplication, RoomID: roomIDFixture(1), MessageID: messageIDFixture(2), Sender: sender, Sequence: 1}
}

func groupKeyFixture(value byte) (key GroupKey) {
	for index := range key {
		key[index] = value
	}
	return key
}

func repeatedNonceFixture(value byte) (nonce Nonce) {
	for index := range nonce {
		nonce[index] = value
	}
	return nonce
}
