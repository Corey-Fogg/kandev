package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/credentiallock"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
)

// repairWorkspaceSession recovers a delegated session that stopped on a
// provider credential failure: it clears a stale account lock left by an
// interrupted refresh and resumes the same conversation. Other failures are
// refused so the coordinator reports them instead of retrying blindly.
func (a *taskCreatorAdapter) repairWorkspaceSession(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	if a.orch == nil {
		return fmt.Errorf("orchestrator unavailable")
	}
	session, err := a.repairableSession(ctx, task, command.SessionID)
	if err != nil {
		return err
	}
	if err := a.clearStaleCredentialLock(ctx, session); err != nil {
		return err
	}
	if _, err := a.orch.RecoverSession(ctx, task.ID, session.ID, "resume"); err != nil {
		return fmt.Errorf("resume session: %w", err)
	}
	return nil
}

func (a *taskCreatorAdapter) repairableSession(ctx context.Context, task *models.Task, sessionID string) (*models.TaskSession, error) {
	sessions, err := a.taskSvc.ListTaskSessions(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	var session *models.TaskSession
	for _, candidate := range sessions {
		if sessionID != "" && candidate.ID != sessionID {
			continue
		}
		if session == nil || candidate.UpdatedAt.After(session.UpdatedAt) {
			session = candidate
		}
	}
	if session == nil {
		return nil, fmt.Errorf("session must belong to this task")
	}
	if !credentiallock.IsLoginFailure(session.ErrorMessage) {
		return nil, fmt.Errorf("repair_session only recovers provider login failures; inspect task_content and report other errors")
	}
	return session, nil
}

// clearStaleCredentialLock removes the account lock directory a provider
// leaves beside its config directory when it exits mid-refresh. Only an
// empty lock older than credentiallock.StaleAge is removed.
func (a *taskCreatorAdapter) clearStaleCredentialLock(ctx context.Context, session *models.TaskSession) error {
	_, err := credentiallock.ClearForProfile(ctx, a.profiles, session.AgentProfileID)
	return err
}
