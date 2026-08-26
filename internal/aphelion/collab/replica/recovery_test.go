package replica

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecoveryEscalatesOnceFromReplayToSnapshotThenClose(t *testing.T) {
	recovery := NewRecovery()
	require.Equal(t, RecoveryReplay, recovery.Action())
	recovery.Failed(RecoveryReplay)
	require.Equal(t, RecoverySnapshot, recovery.Action())
	recovery.Failed(RecoverySnapshot)
	require.Equal(t, RecoveryClose, recovery.Action())
	recovery.Failed(RecoverySnapshot)
	require.Equal(t, RecoveryClose, recovery.Action())
}

func TestRecoverySuccessResetsReplayBudget(t *testing.T) {
	recovery := NewRecovery()
	recovery.Failed(RecoveryReplay)
	recovery.Succeeded()
	require.Equal(t, RecoveryReplay, recovery.Action())
}
