package routingerr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClaudeRefreshContentionIsTransient(t *testing.T) {
	err := Classify(Input{Phase: PhasePromptSend, ProviderID: "claude-acp", Stderr: "Failed to refresh OAuth token: another Claude Code process is refreshing it or exited mid-refresh"})
	require.Equal(t, DecisionShortRetry, Decide(ContextKanban, err, time.Now()))
}
