package backendapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

type recordingGuardSetter struct{ guard executor.DispatchGuard }

func (r *recordingGuardSetter) SetDispatchGuard(guard executor.DispatchGuard) { r.guard = guard }

func TestCoordinatorDispatchGuardIsInstalledWithOrchestrationDisabled(t *testing.T) {
	_, tasks, repo, taskID := coordinatorConversationFixture(t)
	ctx := context.Background()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	setter := &recordingGuardSetter{}
	services := &Services{Orchestration: &orchestrationruntime.Service{Repo: repo}}
	require.True(t, startOrchestrationRuntime(ctx, &config.Config{}, services, setter, &Repositories{Orchestration: repo}, nil, func(func() error) {}, log))
	require.NotNil(t, setter.guard, "the guard is installed before the feature check")
	task, err := tasks.GetTask(ctx, taskID)
	require.NoError(t, err)
	require.ErrorIs(t, setter.guard(ctx, task, nil, "profile"), orchestrationruntime.ErrOrchestrationDisabled)
}
