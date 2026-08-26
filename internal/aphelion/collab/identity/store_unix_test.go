//go:build !windows

package identity

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRestrictedFileStoreUsesPrivatePermissions(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "secrets")
	store, err := NewPlatformStore(directory)
	require.NoError(t, err)
	name := "identity/0123456789abcdef0123456789abcdef"
	require.NoError(t, store.Put(context.Background(), name, []byte("private-test-material")))

	directoryInfo, err := os.Stat(directory)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), directoryInfo.Mode().Perm())
	recordPath := filepath.Join(directory, "identity", "0123456789abcdef0123456789abcdef.secret")
	recordInfo, err := os.Stat(recordPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), recordInfo.Mode().Perm())
	loaded, err := store.Get(context.Background(), name)
	require.NoError(t, err)
	require.Equal(t, []byte("private-test-material"), loaded)
}

func TestRestrictedFileStoreRejectsBroadPermissions(t *testing.T) {
	directory := t.TempDir()
	store, err := NewPlatformStore(directory)
	require.NoError(t, err)
	name := "session/0123456789abcdef0123456789abcdef"
	require.NoError(t, store.Put(context.Background(), name, []byte("secret")))
	recordPath := filepath.Join(directory, "session", "0123456789abcdef0123456789abcdef.secret")
	require.NoError(t, os.Chmod(recordPath, 0o640))

	_, err = store.Get(context.Background(), name)
	require.ErrorContains(t, err, "permissions are too broad")
}

func TestRestrictedFileStoreRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	store, err := NewPlatformStore(directory)
	require.NoError(t, err)
	target := filepath.Join(directory, "target")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0o600))
	namespace := filepath.Join(directory, "group")
	require.NoError(t, os.Mkdir(namespace, 0o700))
	name := "group/0123456789abcdef0123456789abcdef"
	require.NoError(t, os.Symlink(target, filepath.Join(namespace, "0123456789abcdef0123456789abcdef.secret")))

	_, err = store.Get(context.Background(), name)
	require.ErrorContains(t, err, "not a regular file")
	require.ErrorContains(t, store.Delete(context.Background(), name), "not a regular file")
}
