package protocolv2

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

var (
	ErrSignatureInvalid  = errors.New("protocol v2 signature is invalid")
	ErrSenderKeyMismatch = errors.New("protocol v2 sender key does not match signer")
	ErrDecryptionFailed  = errors.New("protocol v2 payload decryption failed")
)

type GroupKey [chacha20poly1305.KeySize]byte

func Seal(entropy io.Reader, key GroupKey, signer ed25519.PrivateKey, header Header, plaintext []byte) (Envelope, error) {
	if len(signer) != ed25519.PrivateKeySize {
		return Envelope{}, fmt.Errorf("protocol v2 signer private key length is invalid")
	}
	publicKey := signer.Public().(ed25519.PublicKey)
	if subtle.ConstantTimeCompare(header.Sender[:], publicKey) != 1 {
		return Envelope{}, ErrSenderKeyMismatch
	}
	aad, err := header.AuthenticatedBytes()
	if err != nil {
		return Envelope{}, err
	}
	aead, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		return Envelope{}, fmt.Errorf("initialize protocol v2 encryption: %w", err)
	}
	if len(plaintext) == 0 || len(plaintext)+aead.Overhead() > MaxCiphertextBytes {
		return Envelope{}, fmt.Errorf("protocol v2 plaintext length %d is outside bounds", len(plaintext))
	}
	if entropy == nil {
		entropy = rand.Reader
	}
	var nonce Nonce
	if _, err := io.ReadFull(entropy, nonce[:]); err != nil {
		return Envelope{}, fmt.Errorf("generate protocol v2 nonce: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce[:], plaintext, aad)
	signed := signatureInput(aad, nonce, ciphertext)
	encodedSignature := ed25519.Sign(signer, signed)
	var signature Signature
	copy(signature[:], encodedSignature)
	return Envelope{Header: header, Nonce: nonce, Ciphertext: ciphertext, Signature: signature}, nil
}

func Open(key GroupKey, expectedSigner ed25519.PublicKey, envelope Envelope) ([]byte, error) {
	if len(expectedSigner) != ed25519.PublicKeySize {
		return nil, ErrSignatureInvalid
	}
	if subtle.ConstantTimeCompare(envelope.Header.Sender[:], expectedSigner) != 1 {
		return nil, ErrSenderKeyMismatch
	}
	aad, err := envelope.Header.AuthenticatedBytes()
	if err != nil {
		return nil, err
	}
	if len(envelope.Ciphertext) < chacha20poly1305.Overhead || len(envelope.Ciphertext) > MaxCiphertextBytes {
		return nil, ErrDecryptionFailed
	}
	signed := signatureInput(aad, envelope.Nonce, envelope.Ciphertext)
	if !ed25519.Verify(expectedSigner, signed, envelope.Signature[:]) {
		return nil, ErrSignatureInvalid
	}
	aead, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		return nil, fmt.Errorf("initialize protocol v2 decryption: %w", err)
	}
	plaintext, err := aead.Open(nil, envelope.Nonce[:], envelope.Ciphertext, aad)
	if err != nil {
		return nil, ErrDecryptionFailed
	}
	return plaintext, nil
}

func VerifyEnvelopeSignature(envelope Envelope) error {
	aad, err := envelope.Header.AuthenticatedBytes()
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(envelope.Header.Sender[:]), signatureInput(aad, envelope.Nonce, envelope.Ciphertext), envelope.Signature[:]) {
		return ErrSignatureInvalid
	}
	return nil
}

func signatureInput(aad []byte, nonce Nonce, ciphertext []byte) []byte {
	result := make([]byte, 0, len(aad)+len(nonce)+len(ciphertext))
	result = append(result, aad...)
	result = append(result, nonce[:]...)
	result = append(result, ciphertext...)
	return result
}
