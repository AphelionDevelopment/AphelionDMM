package relayclient

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParticipantExecutorExposesTerminalTransportFailure(t *testing.T) {
	execution := &ParticipantExecutor{}
	expected := errors.New("relay unavailable")

	execution.fail(expected)

	require.ErrorIs(t, execution.TerminalError(), expected)
}
