package runtime

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantAuthorityModeCannotBeChangedByRuntime(t *testing.T) {
	s, _, task := newRuntime(t)
	router, token, runID := assistantRuntimeCallerMode(t, s, task, "inspect")
	response := runtimeRequest(t, router, "PUT", "/api/v1/orchestration/assistant", token, runID,
		map[string]any{"orchestrator_id": "chief", "execution_mode": "execute", "expected_version": 1})
	require.Equal(t, 403, response.Code)
	binding, err := s.Repo.AssistantBinding(context.Background(), "owner")
	require.NoError(t, err)
	require.Equal(t, "inspect", binding.ExecutionMode)
}

type testAssistantAuthority struct {
	revision    string
	unavailable bool
	beforeRead  func()
}

func (a *testAssistantAuthority) ResolveAssistantAuthority(context.Context, models.AssistantBinding, string, string) (models.AssistantAuthority, error) {
	if a.beforeRead != nil {
		a.beforeRead()
	}
	row := models.AssistantAuthority{Revision: a.revision, Restriction: "claude-broker-v1"}
	if a.unavailable {
		row.UnsupportedReason = "unsupported_profile"
	}
	return row, nil
}

func TestAssistantBindingDefaultsToExecute(t *testing.T) {
	s, _, task := newRuntime(t)
	assistantRuntimeCallerMode(t, s, task, "")
	binding, err := s.Repo.AssistantBinding(context.Background(), "owner")
	require.NoError(t, err)
	require.Equal(t, "execute", binding.ExecutionMode)
}

func TestCoordinatorSessionRequiresBrokerPolicyAndClaimedRun(t *testing.T) {
	s, db, task := newRuntime(t)
	_, _, _ = assistantRuntimeCaller(t, s, task)
	ctx := context.Background()
	session := &taskmodels.TaskSession{ID: "session", TaskID: task}
	require.ErrorIs(t, s.CheckCoordinatorSession(ctx, task, session), models.ErrConflict, "an unrestricted session cannot join the run")
	session.Metadata = map[string]any{mcpprofile.BrokerPolicyMetadataKey: string(mcpprofile.SurfaceOrchestratorBroker)}
	require.NoError(t, s.CheckCoordinatorSession(ctx, task, session))
	session.Metadata = map[string]any{"assistant_broker_policy": "assistant-broker-v1"}
	require.NoError(t, s.CheckCoordinatorSession(ctx, task, session), "legacy broker sessions stay admitted")
	foreign := &taskmodels.TaskSession{ID: "other", TaskID: task, Metadata: session.Metadata}
	require.ErrorIs(t, s.CheckCoordinatorSession(ctx, task, foreign), models.ErrConflict, "a session without the claimed run is refused")
	_, err := db.Exec(`INSERT INTO orchestration_conversation_intents(task_id,revision) VALUES(?,1)
		ON CONFLICT(task_id) DO UPDATE SET revision=revision+1`, task)
	require.NoError(t, err)
	require.ErrorIs(t, s.CheckCoordinatorSession(ctx, task, session), models.ErrConflict, "a superseded intent is refused")
	s.Enabled = false
	require.ErrorIs(t, s.CheckCoordinatorSession(ctx, task, session), ErrOrchestrationDisabled)
}
