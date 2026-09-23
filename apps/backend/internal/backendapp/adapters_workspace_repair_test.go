package backendapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type repairProfiles struct{ dir string }

func (p repairProfiles) GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error) {
	return &settingsmodels.AgentProfile{EnvVars: []settingsmodels.ProfileEnvVar{{Key: claudeConfigDirEnv, Value: p.dir}}}, nil
}

func TestRepairClassifiesOnlyCredentialFailures(t *testing.T) {
	require.True(t, isCredentialFailure("Internal error: Failed to refresh OAuth token: another Claude Code process is refreshing it"))
	require.True(t, isCredentialFailure(`{"type":"authentication_error"}`))
	require.False(t, isCredentialFailure("git push rejected"))
	require.False(t, isCredentialFailure(""))
}

func TestRepairClearsOnlyStaleEmptyCredentialLock(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".claude-work")
	lock := dir + ".lock"
	adapter := &taskCreatorAdapter{profiles: repairProfiles{dir: dir}}
	session := &models.TaskSession{AgentProfileID: "work"}
	ctx := context.Background()

	require.NoError(t, os.Mkdir(lock, 0o755))
	require.NoError(t, adapter.clearStaleCredentialLock(ctx, session))
	require.DirExists(t, lock, "a fresh lock may belong to a live refresh")

	old := time.Now().Add(-2 * staleCredentialLockAge)
	require.NoError(t, os.WriteFile(filepath.Join(lock, "owner"), nil, 0o600))
	require.NoError(t, os.Chtimes(lock, old, old))
	require.NoError(t, adapter.clearStaleCredentialLock(ctx, session))
	require.DirExists(t, lock, "a lock with contents is never removed")

	require.NoError(t, os.Remove(filepath.Join(lock, "owner")))
	require.NoError(t, os.Chtimes(lock, old, old))
	require.NoError(t, adapter.clearStaleCredentialLock(ctx, session))
	require.NoDirExists(t, lock)

	require.NoError(t, adapter.clearStaleCredentialLock(ctx, session), "no lock is a no-op")
}
