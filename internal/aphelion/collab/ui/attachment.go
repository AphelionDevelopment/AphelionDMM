package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sdmm/internal/aphelion/collab/executor"
	"sdmm/internal/aphelion/collab/model"
)

const incompleteSessionCleanupTimeout = 5 * time.Second

var ErrAttachmentTargetChanged = errors.New("collaboration attachment target changed")

type LocalSessionLifecycle interface {
	CreateLocal(context.Context, model.Snapshot) error
	Leave(context.Context) error
}

type CollaborationExecutorProvider interface {
	CollaborationExecutor() executor.Executor
}

type CollaborationAttachmentTarget interface {
	AttachCollaborationExecutor(executor.Executor) error
}

// PrepareLocalSession creates a session and returns its synchronized executor for UI-thread attachment.
func PrepareLocalSession(ctx context.Context, lifecycle LocalSessionLifecycle, provider CollaborationExecutorProvider, snapshot model.Snapshot) (executor.Executor, error) {
	if lifecycle == nil || provider == nil {
		return nil, fmt.Errorf("prepare local collaboration session: lifecycle is unavailable")
	}
	if err := lifecycle.CreateLocal(ctx, snapshot); err != nil {
		return nil, err
	}
	execution := provider.CollaborationExecutor()
	if execution != nil {
		return execution, nil
	}
	cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), incompleteSessionCleanupTimeout)
	defer cancelCleanup()
	cleanupErr := lifecycle.Leave(cleanupContext)
	return nil, errors.Join(fmt.Errorf("prepare local collaboration session: synchronized executor is unavailable"), cleanupErr)
}

// AttachPreparedSession installs a synchronized executor only if the originating editor is still current.
func AttachPreparedSession(execution executor.Executor, target CollaborationAttachmentTarget, stillCurrent bool) error {
	if !stillCurrent {
		return ErrAttachmentTargetChanged
	}
	if execution == nil || target == nil {
		return fmt.Errorf("attach prepared collaboration session: target or executor is unavailable")
	}
	return target.AttachCollaborationExecutor(execution)
}
