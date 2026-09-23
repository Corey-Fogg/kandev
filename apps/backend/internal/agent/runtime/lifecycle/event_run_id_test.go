package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrchestrationEventsCarryLaunchRunID(t *testing.T) {
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session"}
	execution.setRuntimeEnvironment(map[string]string{
		"KANDEV_RUN_ID": "run-1", "KANDEV_RUN_TOKEN": "secret", "KANDEV_RUNTIME_API_PREFIX": "/api/v1/orchestration",
	})
	require.Equal(t, "run-1", newAgentEventPayload(execution).RunID)
	execution.clearRuntimeEnvironment()
	require.Equal(t, "run-1", newAgentEventPayload(execution).RunID, "run identity outlives credential teardown")

	execution.RunID = "owned-run"
	require.Equal(t, "owned-run", newAgentEventPayload(execution).RunID, "a run-owned execution keeps its own identity")
}

func TestOfficeTaskEventsKeepUpstreamRunIdentity(t *testing.T) {
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session"}
	execution.setRuntimeEnvironment(map[string]string{"KANDEV_RUN_ID": "office-run", "KANDEV_RUNTIME_API_PREFIX": "/api/v1/office"})
	require.Empty(t, newAgentEventPayload(execution).RunID)
}
