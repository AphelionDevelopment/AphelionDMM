//go:build windows

package identity

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDPAPIStoreProtectsPlaintextAndScopesRecords(t *testing.T) {
	directory := t.TempDir()
	store, err := NewPlatformStore(directory)
	require.NoError(t, err)
	plaintext := bytes.Repeat([]byte("private-test-material"), 8)

	require.NoError(t, store.Put(context.Background(), "identity/0123456789abcdef0123456789abcdef", plaintext))
	entries, err := os.ReadDir(filepath.Join(directory, "identity"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	ciphertext, err := os.ReadFile(filepath.Join(directory, "identity", entries[0].Name()))
	require.NoError(t, err)
	require.NotContains(t, ciphertext, plaintext)

	loaded, err := store.Get(context.Background(), "identity/0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	require.Equal(t, plaintext, loaded)

	otherPath := filepath.Join(directory, "identity", "fedcba9876543210fedcba9876543210.secret")
	require.NoError(t, os.WriteFile(otherPath, ciphertext, 0o600))
	_, err = store.Get(context.Background(), "identity/fedcba9876543210fedcba9876543210")
	require.ErrorContains(t, err, "unprotect secret")
}

func TestDPAPIStoreRejectsInvalidRecordNames(t *testing.T) {
	store, err := NewPlatformStore(t.TempDir())
	require.NoError(t, err)

	for _, name := range []string{"", "identity", "../escape", "identity/short", "unknown/0123456789abcdef0123456789abcdef"} {
		require.Error(t, store.Put(context.Background(), name, []byte("value")), name)
	}
}

func TestDPAPIStoreDeleteIsExplicit(t *testing.T) {
	store, err := NewPlatformStore(t.TempDir())
	require.NoError(t, err)
	name := "group/0123456789abcdef0123456789abcdef"
	require.NoError(t, store.Put(context.Background(), name, []byte("value")))
	require.NoError(t, store.Delete(context.Background(), name))
	_, err = store.Get(context.Background(), name)
	require.ErrorIs(t, err, ErrSecretNotFound)
	require.ErrorIs(t, store.Delete(context.Background(), name), ErrSecretNotFound)
}
