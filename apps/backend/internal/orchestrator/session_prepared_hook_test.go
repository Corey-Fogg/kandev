package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestStartTaskWithRouteBindsPreparedSessionBeforeLaunch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "routed-task", "routed-old-session", models.TaskSessionStateCompleted)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "routed-task", v1.TaskStateInProgress)
	bound := ""
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			require.NotEmpty(t, bound, "the session must be bound before the agent launches")
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	launch := executor.LaunchContext{Prompt: "go", OnSessionPrepared: func(_ context.Context, sessionID string) error {
		bound = sessionID
		return nil
	}}
	execution, err := svc.StartTaskWithRoute(ctx, "routed-task", "profile-1", launch, executor.RouteOverride{ExecutionProfileID: "profile-1"})
	require.NoError(t, err)
	require.NotNil(t, execution)
	require.NotEmpty(t, bound)
	require.Equal(t, execution.SessionID, bound)
}

func TestStartTaskWithRouteStopsWhenSessionBindingFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "routed-denied", "routed-denied-old", models.TaskSessionStateCompleted)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "routed-denied", v1.TaskStateInProgress)
	launched := false
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launched = true
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	denied := errors.New("authority revoked")
	launch := executor.LaunchContext{Prompt: "go", OnSessionPrepared: func(context.Context, string) error { return denied }}
	_, err := svc.StartTaskWithRoute(ctx, "routed-denied", "profile-1", launch, executor.RouteOverride{ExecutionProfileID: "profile-1"})
	require.ErrorIs(t, err, denied)
	require.False(t, launched)
}
