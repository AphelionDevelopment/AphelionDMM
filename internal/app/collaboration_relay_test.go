package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitializeClientCollaborationPersistsIdentityReference(t *testing.T) {
	config := newCollaborationConfig()
	first, err := initializeClientCollaboration(t.TempDir(), config)
	require.NoError(t, err)
	require.NotEmpty(t, config.IdentitySecretRef)
	firstPublicKey := first.identity.PublicKey
	require.NoError(t, first.store.Close())

	second, err := initializeClientCollaboration(first.internalDir, config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.store.Close() })
	require.Equal(t, firstPublicKey, second.identity.PublicKey)
}
