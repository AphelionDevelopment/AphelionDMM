package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollaborationConfigDefaultsToPublicRelay(t *testing.T) {
	config := newCollaborationConfig()
	require.Equal(t, uint(1), config.Version)
	require.Equal(t, "https://mapping.a13.info", config.RelayURL)
	require.NoError(t, config.Validate())
}

func TestCollaborationConfigRejectsCleartextNonLoopback(t *testing.T) {
	config := newCollaborationConfig()
	config.RelayURL = "http://example.invalid"
	require.ErrorContains(t, config.Validate(), "HTTPS")
	config.RelayURL = "http://127.0.0.1:8080"
	require.NoError(t, config.Validate())
}
