package credentiallock

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

type profiles struct{ dir string }

func (p profiles) GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error) {
	return &settingsmodels.AgentProfile{EnvVars: []settingsmodels.ProfileEnvVar{{Key: ConfigDirEnv, Value: p.dir}}}, nil
}

func age(t *testing.T, path string, by time.Duration) {
	t.Helper()
	at := time.Now().Add(-by)
	require.NoError(t, os.Chtimes(path, at, at))
}

func TestClearForProfileRemovesOnlyStaleEmptyLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".claude-work")
	lock := dir + ".lock"
	ctx := context.Background()
	reader := profiles{dir: dir}

	cleared, err := ClearForProfile(ctx, reader, "work")
	require.NoError(t, err, "no lock is a no-op")
	require.False(t, cleared)

	require.NoError(t, os.Mkdir(lock, 0o755))
	cleared, err = ClearForProfile(ctx, reader, "work")
	require.NoError(t, err)
	require.False(t, cleared)
	require.DirExists(t, lock, "a fresh lock may belong to a live refresh")

	// A holder touches its lock every five seconds, so one it refreshed
	// just inside the stale window is still live.
	age(t, lock, StaleAge-5*time.Second)
	cleared, err = ClearForProfile(ctx, reader, "work")
	require.NoError(t, err)
	require.False(t, cleared)
	require.DirExists(t, lock)

	require.NoError(t, os.WriteFile(filepath.Join(lock, "owner"), nil, 0o600))
	age(t, lock, 2*StaleAge)
	cleared, err = ClearForProfile(ctx, reader, "work")
	require.NoError(t, err)
	require.False(t, cleared)
	require.DirExists(t, lock, "a lock with contents is never removed")

	require.NoError(t, os.Remove(filepath.Join(lock, "owner")))
	age(t, lock, StaleAge+time.Second)
	cleared, err = ClearForProfile(ctx, reader, "work")
	require.NoError(t, err)
	require.True(t, cleared)
	require.NoDirExists(t, lock)
}

func TestClearStaleIgnoresLockFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".claude")
	require.NoError(t, os.WriteFile(dir+".lock", nil, 0o600))
	age(t, dir+".lock", 2*StaleAge)
	cleared, err := ClearStale(dir)
	require.NoError(t, err)
	require.False(t, cleared)
	require.FileExists(t, dir+".lock")
}

func TestConfigDirPrefersAbsoluteProfileDir(t *testing.T) {
	profile := &settingsmodels.AgentProfile{EnvVars: []settingsmodels.ProfileEnvVar{{Key: ConfigDirEnv, Value: "/accounts/personal"}}}
	require.Equal(t, "/accounts/personal", ConfigDir(profile))
	relative := &settingsmodels.AgentProfile{EnvVars: []settingsmodels.ProfileEnvVar{{Key: ConfigDirEnv, Value: "personal"}}}
	require.Equal(t, ".claude", filepath.Base(ConfigDir(relative)))
	require.Empty(t, ConfigDir(nil))
}
