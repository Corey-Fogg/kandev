package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/credentiallock"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// The production failure text: the terminal copy is sanitized, so
// "token: another" arrives as "token: ***".
const refreshContentionSanitized = "Internal error: Failed to refresh OAuth token: *** Claude Code process is refreshing it or exited mid-refresh. This is usually transient; retry in a minute, and if it persists close other Claude Code processes or sign in again"

type lockProfiles map[string]string

func (p lockProfiles) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	return &settingsmodels.AgentProfile{ID: id, EnvVars: []settingsmodels.ProfileEnvVar{{Key: credentiallock.ConfigDirEnv, Value: p[id]}}}, nil
}

// A killed refresh leaves an empty lock that fails every later turn. The
// first launch must not touch a lock a live refresh may hold; by the third
// retry (15+30+60 s later) the orphaned lock is past the stale age and the
// launch clears it before the agent starts.
func TestCoordinatorLaunchesClearStaleCredentialLock(t *testing.T) {
	s, db, task := newRuntime(t)
	ctx := context.Background()
	s.FailureHandlerInstalled = true
	dir := filepath.Join(t.TempDir(), ".claude-personal")
	lock := dir + ".lock"
	require.NoError(t, os.Mkdir(lock, 0o755))
	profiles := lockProfiles{"personal": dir}
	var cleared []string
	s.ClearCredentialLock = func(ctx context.Context, profile string) {
		cleared = append(cleared, profile)
		_, err := credentiallock.ClearForProfile(ctx, profiles, profile)
		require.NoError(t, err)
	}
	var lockAtStart []bool
	s.Start = func(ctx context.Context, l Launch) error {
		require.NoError(t, l.OnSessionPrepared(ctx, "session"))
		_, err := os.Stat(lock)
		lockAtStart = append(lockAtStart, err == nil)
		return nil
	}
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "example", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	_, err = s.Process(ctx, run)
	require.NoError(t, err)

	lockAge := []time.Duration{15 * time.Second, 45 * time.Second, 105 * time.Second}
	for attempt := 1; attempt <= len(lockAge); attempt++ {
		failure := watcher.AgentEventData{RunID: run.ID, TaskID: task, SessionID: "session", AgentID: "claude-acp", AgentExecutionID: fmt.Sprintf("execution-%d", attempt), PromptGeneration: 1, EvidenceKnown: true, ErrorMessage: refreshContentionSanitized}
		got, retryAt, err := s.HandleFailure(ctx, failure)
		require.NoError(t, err)
		require.Equal(t, attempt, got, "a credential refresh failure before any output is retried")
		require.True(t, retryAt.After(time.Now()))
		if _, statErr := os.Stat(lock); statErr == nil {
			at := time.Now().Add(-lockAge[attempt-1])
			require.NoError(t, os.Chtimes(lock, at, at))
		}
		_, err = db.Exec(`UPDATE runs SET scheduled_retry_at=? WHERE id=?`, time.Now().Add(-time.Second), run.ID)
		require.NoError(t, err)
		retried, err := s.Runs.ClaimNextEligibleRun(ctx)
		require.NoError(t, err)
		require.Equal(t, run.ID, retried.ID)
		_, err = s.Process(ctx, retried)
		require.NoError(t, err)
	}
	require.Equal(t, []bool{true, true, true, false}, lockAtStart)
	require.Equal(t, []string{"personal", "personal", "personal", "personal"}, cleared)
	require.NoDirExists(t, lock)
}
