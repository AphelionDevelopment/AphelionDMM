package identity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagerLoadOrCreatePersistsStableIdentity(t *testing.T) {
	store := newMemoryStore()
	manager := NewManager(store, bytes.NewReader(bytes.Repeat([]byte{0x31}, 256)))

	created, reference, err := manager.LoadOrCreate(context.Background(), "")
	require.NoError(t, err)
	require.NotEmpty(t, reference)
	require.Len(t, created.PrivateKey, ed25519.PrivateKeySize)
	require.Equal(t, created.PrivateKey.Public().(ed25519.PublicKey), ed25519.PublicKey(created.PublicKey[:]))

	loaded, loadedReference, err := manager.LoadOrCreate(context.Background(), reference)
	require.NoError(t, err)
	require.Equal(t, reference, loadedReference)
	require.Equal(t, created, loaded)
	require.Equal(t, 1, store.putCount)
}

func TestManagerRejectsCorruptIdentityWithoutReplacingIt(t *testing.T) {
	store := newMemoryStore()
	store.values["identity/00000000000000000000000000000000"] = []byte("corrupt")
	manager := NewManager(store, bytes.NewReader(bytes.Repeat([]byte{0x41}, 256)))

	_, _, err := manager.LoadOrCreate(context.Background(), "identity/00000000000000000000000000000000")
	require.ErrorContains(t, err, "invalid identity secret")
	require.Equal(t, 0, store.putCount)
	require.Equal(t, []byte("corrupt"), store.values["identity/00000000000000000000000000000000"])
}

func TestManagerClearsLoadedPrivateKeyBuffer(t *testing.T) {
	store := newMemoryStore()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x51}, ed25519.SeedSize))
	reference := SecretRef("identity/11111111111111111111111111111111")
	store.values[string(reference)] = append([]byte(nil), privateKey...)

	_, _, err := NewManager(store, nil).LoadOrCreate(context.Background(), reference)
	require.NoError(t, err)
	require.Equal(t, make([]byte, ed25519.PrivateKeySize), store.lastReturned)
}

func TestManagerCreatesSeparatedSessionSecrets(t *testing.T) {
	store := newMemoryStore()
	random := append(bytes.Repeat([]byte{0x61}, 48), bytes.Repeat([]byte{0x62}, 48)...)
	manager := NewManager(store, bytes.NewReader(random))

	sessionKey, sessionReference, err := manager.CreateSessionKey(context.Background(), SessionKeyNamespace)
	require.NoError(t, err)
	groupKey, groupReference, err := manager.CreateSessionKey(context.Background(), GroupKeyNamespace)
	require.NoError(t, err)

	require.NotEqual(t, sessionReference, groupReference)
	require.Contains(t, string(sessionReference), "session/")
	require.Contains(t, string(groupReference), "group/")
	require.NotEqual(t, sessionKey, groupKey)
	require.Equal(t, sessionKey[:], store.values[string(sessionReference)])
	require.Equal(t, groupKey[:], store.values[string(groupReference)])
}

func TestManagerHonorsCancellationBeforeMutation(t *testing.T) {
	store := newMemoryStore()
	manager := NewManager(store, bytes.NewReader(bytes.Repeat([]byte{0x71}, 256)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := manager.LoadOrCreate(ctx, "")
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, store.values)
}

func TestManagerDoesNotExposeSecretReferenceOnStoreFailure(t *testing.T) {
	store := newMemoryStore()
	store.putError = errors.New("disk unavailable")
	manager := NewManager(store, bytes.NewReader(bytes.Repeat([]byte{0x81}, 256)))

	_, reference, err := manager.CreateSessionKey(context.Background(), SessionKeyNamespace)
	require.Error(t, err)
	require.Empty(t, reference)
	require.NotContains(t, err.Error(), "81")
}

type memoryStore struct {
	values       map[string][]byte
	lastReturned []byte
	putError     error
	putCount     int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{values: make(map[string][]byte)}
}

func (store *memoryStore) Put(ctx context.Context, name string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if store.putError != nil {
		return store.putError
	}
	store.putCount++
	store.values[name] = append([]byte(nil), value...)
	return nil
}

func (store *memoryStore) Get(ctx context.Context, name string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, ok := store.values[name]
	if !ok {
		return nil, ErrSecretNotFound
	}
	store.lastReturned = append([]byte(nil), value...)
	return store.lastReturned, nil
}

func (store *memoryStore) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := store.values[name]; !ok {
		return ErrSecretNotFound
	}
	delete(store.values, name)
	return nil
}
