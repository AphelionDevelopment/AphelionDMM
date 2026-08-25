package client

import (
	"fmt"
	"sync"
)

type State string

const (
	StateDisconnected  State = "disconnected"
	StateConnecting    State = "connecting"
	StateSynchronizing State = "synchronizing"
	StateCaughtUp      State = "caught_up"
	StateReconnecting  State = "reconnecting"
	StateReadOnly      State = "read_only"
	StateConflict      State = "conflict"
	StateClosed        State = "closed"
)

type Event string

const (
	EventConnect        Event = "connect"
	EventConnected      Event = "connected"
	EventSynchronized   Event = "synchronized"
	EventReadOnly       Event = "read_only"
	EventConflict       Event = "conflict"
	EventResolved       Event = "resolved"
	EventConnectionLost Event = "connection_lost"
	EventClose          Event = "close"
)

var transitions = map[State]map[Event]State{
	StateDisconnected: {EventConnect: StateConnecting, EventClose: StateClosed},
	StateConnecting:   {EventConnected: StateSynchronizing, EventConnectionLost: StateDisconnected, EventClose: StateClosed},
	StateSynchronizing: {
		EventSynchronized:   StateCaughtUp,
		EventReadOnly:       StateReadOnly,
		EventConflict:       StateConflict,
		EventConnectionLost: StateReconnecting,
		EventClose:          StateClosed,
	},
	StateCaughtUp: {
		EventConflict:       StateConflict,
		EventReadOnly:       StateReadOnly,
		EventConnectionLost: StateReconnecting,
		EventClose:          StateClosed,
	},
	StateReconnecting: {EventConnected: StateSynchronizing, EventClose: StateClosed},
	StateReadOnly:     {EventConnectionLost: StateReconnecting, EventClose: StateClosed},
	StateConflict:     {EventResolved: StateCaughtUp, EventConnectionLost: StateReconnecting, EventClose: StateClosed},
}

func Reduce(state State, event Event) (State, error) {
	events, exists := transitions[state]
	if !exists {
		return state, fmt.Errorf("state %q accepts no transitions", state)
	}
	next, exists := events[event]
	if !exists {
		return state, fmt.Errorf("event %q is invalid in state %q", event, state)
	}
	return next, nil
}

type StateMachine struct {
	mutex sync.RWMutex
	state State
}

func NewStateMachine() *StateMachine {
	return &StateMachine{state: StateDisconnected}
}

func (machine *StateMachine) State() State {
	machine.mutex.RLock()
	defer machine.mutex.RUnlock()
	return machine.state
}

func (machine *StateMachine) Apply(event Event) error {
	machine.mutex.Lock()
	defer machine.mutex.Unlock()
	next, err := Reduce(machine.state, event)
	if err != nil {
		return err
	}
	machine.state = next
	return nil
}
