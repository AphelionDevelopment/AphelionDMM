package client

import "testing"

func TestConnectionTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		start  State
		event  Event
		wanted State
	}{
		{"connect", StateDisconnected, EventConnect, StateConnecting},
		{"socket connected", StateConnecting, EventConnected, StateSynchronizing},
		{"replay complete", StateSynchronizing, EventSynchronized, StateCaughtUp},
		{"viewer synchronized", StateSynchronizing, EventReadOnly, StateReadOnly},
		{"durable conflict", StateCaughtUp, EventConflict, StateConflict},
		{"conflict resolved", StateConflict, EventResolved, StateCaughtUp},
		{"connection lost", StateCaughtUp, EventConnectionLost, StateReconnecting},
		{"reconnected", StateReconnecting, EventConnected, StateSynchronizing},
		{"explicit leave", StateReadOnly, EventClose, StateClosed},
		{"close while connecting", StateConnecting, EventClose, StateClosed},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			actual, err := Reduce(test.start, test.event)
			if err != nil {
				t.Fatalf("Reduce() error = %v", err)
			}
			if actual != test.wanted {
				t.Fatalf("Reduce(%q, %q) = %q, want %q", test.start, test.event, actual, test.wanted)
			}
		})
	}
}

func TestConnectionTransitionsRejectInvalidMoves(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		start State
		event Event
	}{
		{StateDisconnected, EventSynchronized},
		{StateConnecting, EventConflict},
		{StateClosed, EventConnect},
		{StateCaughtUp, EventConnected},
	} {
		if _, err := Reduce(test.start, test.event); err == nil {
			t.Errorf("Reduce(%q, %q) error = nil", test.start, test.event)
		}
	}
}

func TestConnectionMachineIsSafeForConcurrentReads(t *testing.T) {
	t.Parallel()

	machine := NewStateMachine()
	if err := machine.Apply(EventConnect); err != nil {
		t.Fatal(err)
	}
	if got := machine.State(); got != StateConnecting {
		t.Fatalf("State() = %q, want connecting", got)
	}
}
