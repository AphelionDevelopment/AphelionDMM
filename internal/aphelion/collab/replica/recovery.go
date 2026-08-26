package replica

import "sync"

type RecoveryAction string

const (
	RecoveryReplay   RecoveryAction = "request_replay"
	RecoverySnapshot RecoveryAction = "request_snapshot"
	RecoveryClose    RecoveryAction = "close_with_diagnostic"
)

type Recovery struct {
	mutex  sync.Mutex
	action RecoveryAction
}

func NewRecovery() *Recovery {
	return &Recovery{action: RecoveryReplay}
}

func (recovery *Recovery) Action() RecoveryAction {
	recovery.mutex.Lock()
	defer recovery.mutex.Unlock()
	return recovery.action
}

func (recovery *Recovery) Failed(action RecoveryAction) {
	recovery.mutex.Lock()
	defer recovery.mutex.Unlock()
	if action != recovery.action {
		return
	}
	switch recovery.action {
	case RecoveryReplay:
		recovery.action = RecoverySnapshot
	case RecoverySnapshot:
		recovery.action = RecoveryClose
	}
}

func (recovery *Recovery) Succeeded() {
	recovery.mutex.Lock()
	defer recovery.mutex.Unlock()
	recovery.action = RecoveryReplay
}
