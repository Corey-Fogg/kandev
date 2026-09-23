package runtime

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func defaultAssistantRuntime(s *Service) *Service {
	return &Service{Repo: s.Repo, Personas: s.Personas, Runs: s.Runs, Queue: s.Queue,
		Auth: s.Auth, Tasks: s.Tasks, Manager: s.Manager}
}

func bindFeatureTestAssistant(t *testing.T, s *Service, task string) {
	t.Helper()
	require.NoError(t, s.Repo.SelectAssistant(context.Background(), &models.AssistantBinding{
		OwnerUserID: "owner", OrchestratorID: "chief", WorkspaceID: "ws", ConversationID: task,
	}, 0))
}

// @covers AC-ORCHESTRATION-ASSISTANT-010.2, AC-ORCHESTRATION-ASSISTANT-010.3
func TestAssistantFeatureGateRetainsHistoryWithoutAdmittingTurns(t *testing.T) {
	s, _, task := newRuntime(t)
	bindFeatureTestAssistant(t, s, task)
	disabled := defaultAssistantRuntime(s)
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	require.Equal(t, 200, runtimeRequest(t, assistantRouter(disabled), "GET", path, "", "", nil).Code)
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(disabled, "foreign"), "GET", path, "", "", nil).Code)
	response := runtimeRequest(t, assistantRouter(disabled), "POST", path, "", "", map[string]string{"body": "Summarize the example tasks"})
	require.Equal(t, 404, response.Code, response.Body.String())
	rows, err := s.Repo.ListComments(context.Background(), task, 10)
	require.NoError(t, err)
	require.Empty(t, rows)
	owner, err := s.Repo.ConversationUserOwner(context.Background(), task)
	require.NoError(t, err)
	require.Equal(t, "owner", owner)
}

func TestAssistantFeatureGateBlocksQueueAndLaunch(t *testing.T) {
	s, _, task := newRuntime(t)
	bindFeatureTestAssistant(t, s, task)
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "callback", "queued-before-disable", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	disabled := defaultAssistantRuntime(s)
	started := false
	disabled.Start = func(context.Context, Launch) error { started = true; return nil }
	handled, err := disabled.Process(ctx, run)
	require.True(t, handled)
	require.Error(t, err)
	require.False(t, started)
}

func TestAssistantFeatureGateKeepsOrdinaryCoordinatorAvailable(t *testing.T) {
	s, _, task := newRuntime(t)
	disabled := defaultAssistantRuntime(s)
	response := runtimeRequest(t, assistantRouter(disabled), "POST", "/api/v1/orchestration/tasks/"+task+"/comments", "", "", map[string]string{"body": "Summarize the example tasks"})
	require.Equal(t, 201, response.Code, response.Body.String())
}
