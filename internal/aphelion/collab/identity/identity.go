package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"sdmm/internal/aphelion/collab/protocolv2"
)

const (
	IdentityNamespace   = "identity"
	SessionKeyNamespace = "session"
	GroupKeyNamespace   = "group"
	CapabilityNamespace = "capability"
)

type SecretRef string

type Identity struct {
	PublicKey  protocolv2.ActorKey
	PrivateKey ed25519.PrivateKey
}

type Manager struct {
	store  SecretStore
	random io.Reader
}

func NewManager(store SecretStore, random io.Reader) *Manager {
	if random == nil {
		random = rand.Reader
	}
	return &Manager{store: store, random: random}
}

func (manager *Manager) LoadOrCreate(ctx context.Context, reference SecretRef) (Identity, SecretRef, error) {
	if err := ctx.Err(); err != nil {
		return Identity{}, "", err
	}
	if manager == nil || manager.store == nil {
		return Identity{}, "", fmt.Errorf("identity secret store is unavailable")
	}
	if reference == "" {
		return manager.createIdentity(ctx)
	}
	if err := validateSecretReference(string(reference), IdentityNamespace); err != nil {
		return Identity{}, "", fmt.Errorf("identity secret reference is invalid")
	}
	encoded, err := manager.store.Get(ctx, string(reference))
	if err != nil {
		return Identity{}, "", fmt.Errorf("load identity secret: %w", err)
	}
	defer zero(encoded)
	if len(encoded) != ed25519.PrivateKeySize {
		return Identity{}, "", fmt.Errorf("invalid identity secret")
	}
	privateKey := append(ed25519.PrivateKey(nil), encoded...)
	return identityFromPrivateKey(privateKey), reference, nil
}

func (manager *Manager) CreateSessionKey(ctx context.Context, namespace string) ([32]byte, SecretRef, error) {
	var key [32]byte
	if err := ctx.Err(); err != nil {
		return key, "", err
	}
	if manager == nil || manager.store == nil {
		return key, "", fmt.Errorf("identity secret store is unavailable")
	}
	if !validNamespace(namespace) || namespace == IdentityNamespace {
		return key, "", fmt.Errorf("secret namespace is invalid")
	}
	if _, err := io.ReadFull(manager.random, key[:]); err != nil {
		return [32]byte{}, "", fmt.Errorf("generate session secret: %w", err)
	}
	reference, err := manager.newReference(namespace)
	if err != nil {
		zero(key[:])
		return [32]byte{}, "", err
	}
	if err := manager.store.Put(ctx, string(reference), key[:]); err != nil {
		zero(key[:])
		return [32]byte{}, "", fmt.Errorf("store session secret: %w", err)
	}
	return key, reference, nil
}

func (manager *Manager) StoreSessionKey(ctx context.Context, namespace string, key [32]byte) (SecretRef, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if manager == nil || manager.store == nil || !validNamespace(namespace) || namespace == IdentityNamespace || key == ([32]byte{}) {
		return "", fmt.Errorf("session secret is invalid")
	}
	reference, err := manager.newReference(namespace)
	if err != nil {
		return "", err
	}
	if err := manager.store.Put(ctx, string(reference), key[:]); err != nil {
		return "", fmt.Errorf("store session secret: %w", err)
	}
	return reference, nil
}

func (manager *Manager) createIdentity(ctx context.Context) (Identity, SecretRef, error) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := io.ReadFull(manager.random, seed); err != nil {
		return Identity{}, "", fmt.Errorf("generate identity: %w", err)
	}
	defer zero(seed)
	privateKey := ed25519.NewKeyFromSeed(seed)
	reference, err := manager.newReference(IdentityNamespace)
	if err != nil {
		zero(privateKey)
		return Identity{}, "", err
	}
	if err := manager.store.Put(ctx, string(reference), privateKey); err != nil {
		zero(privateKey)
		return Identity{}, "", fmt.Errorf("store identity secret: %w", err)
	}
	return identityFromPrivateKey(privateKey), reference, nil
}

func (manager *Manager) newReference(namespace string) (SecretRef, error) {
	var identifier [16]byte
	if _, err := io.ReadFull(manager.random, identifier[:]); err != nil {
		return "", fmt.Errorf("generate secret reference: %w", err)
	}
	return SecretRef(namespace + "/" + hex.EncodeToString(identifier[:])), nil
}

func identityFromPrivateKey(privateKey ed25519.PrivateKey) Identity {
	publicKey := privateKey.Public().(ed25519.PublicKey)
	var actorKey protocolv2.ActorKey
	copy(actorKey[:], publicKey)
	return Identity{PublicKey: actorKey, PrivateKey: privateKey}
}

func zero(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
