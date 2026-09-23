package backendapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
)

// staleCredentialLockAge is well past the few seconds a provider holds its
// credential lock while refreshing; an older empty lock was left by a
// process that exited mid-refresh.
const staleCredentialLockAge = time.Minute

const claudeConfigDirEnv = "CLAUDE_CONFIG_DIR"

// credentialFailureMarkers identify a session that stopped because its
// provider could not refresh or use the account login.
var credentialFailureMarkers = []string{
	"failed to refresh oauth token",
	"oauth token has expired",
	"authentication_error",
}

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
	if !isCredentialFailure(session.ErrorMessage) {
		return nil, fmt.Errorf("repair_session only recovers provider login failures; inspect task_content and report other errors")
	}
	return session, nil
}

func isCredentialFailure(message string) bool {
	message = strings.ToLower(message)
	for _, marker := range credentialFailureMarkers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// clearStaleCredentialLock removes the account lock directory a provider
// leaves beside its config directory when it exits mid-refresh. Only an
// empty lock older than staleCredentialLockAge is removed.
func (a *taskCreatorAdapter) clearStaleCredentialLock(ctx context.Context, session *models.TaskSession) error {
	configDir, err := a.claudeConfigDir(ctx, session.AgentProfileID)
	if err != nil || configDir == "" {
		return err
	}
	lock := filepath.Clean(configDir) + ".lock"
	info, err := os.Stat(lock)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.IsDir() || time.Since(info.ModTime()) < staleCredentialLockAge {
		return err
	}
	entries, err := os.ReadDir(lock)
	if err != nil || len(entries) != 0 {
		return err
	}
	return os.Remove(lock)
}

func (a *taskCreatorAdapter) claudeConfigDir(ctx context.Context, profileID string) (string, error) {
	if a.profiles == nil || profileID == "" {
		return "", nil
	}
	profile, err := a.profiles.GetAgentProfile(ctx, profileID)
	if err != nil || profile == nil {
		return "", err
	}
	for _, env := range profile.EnvVars {
		if env.Key == claudeConfigDirEnv && filepath.IsAbs(env.Value) {
			return env.Value, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil
	}
	return filepath.Join(home, ".claude"), nil
}
