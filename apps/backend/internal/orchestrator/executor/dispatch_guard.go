package executor

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// DispatchTarget names a native dispatch. Session loads the session row on
// demand and checks it belongs to the task, so a guard that admits a dispatch
// from the task id alone reads nothing more.
type DispatchTarget struct {
	TaskID, SessionID string
	Session           func() (*models.TaskSession, error)
}

// DispatchGuard admits or rejects a native launch, resume, prompt, steer or
// model switch immediately before it reaches the agent. Nil admits every
// dispatch. It is configured once during startup, before dispatch begins.
type DispatchGuard func(context.Context, DispatchTarget) error

func (e *Executor) SetDispatchGuard(guard DispatchGuard) { e.dispatchGuard = guard }

// CheckDispatch runs the dispatch guard. session is the caller's already
// loaded session row, or nil to load it only if the guard needs it.
func (e *Executor) CheckDispatch(ctx context.Context, taskID, sessionID string, session *models.TaskSession) error {
	if e.dispatchGuard == nil {
		return nil
	}
	load := func() (*models.TaskSession, error) {
		if session == nil {
			loaded, err := e.repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				return nil, err
			}
			session = loaded
		}
		if session == nil || session.TaskID != taskID {
			return nil, ErrExecutionNotFound
		}
		return session, nil
	}
	return e.dispatchGuard(ctx, DispatchTarget{TaskID: taskID, SessionID: sessionID, Session: load})
}

func (e *Executor) guardedProcessStart(ctx context.Context, taskID, sessionID, executionID string) error {
	if err := e.CheckDispatch(ctx, taskID, sessionID, nil); err != nil {
		return err
	}
	return e.agentManager.StartAgentProcess(ctx, executionID)
}
