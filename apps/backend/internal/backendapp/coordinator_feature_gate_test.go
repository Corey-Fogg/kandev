package backendapp

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	orchestrationmodels "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestration/personas"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	runstore "github.com/kandev/kandev/internal/runs/repository/sqlite"
	runservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-ORCHESTRATION-COORDINATOR-005.2
func TestOrchestratorFeatureGateNativeRunMatrix(t *testing.T) {
	a, _, repo, taskID := coordinatorConversationFixture(t)
	ctx := context.Background()
	db := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	runs := runstore.NewWithDB(db, db)
	require.NoError(t, runs.Migrate())
	queue := runservice.New(runs, nil, log, nil)
	for _, agent := range []string{"fixture-chief", "office-worker"} {
		_, err := queue.QueueRun(ctx, runservice.QueueRunRequest{AgentProfileID: agent, TaskID: taskID, Reason: "task_comment", IdempotencyKey: "gate-" + agent})
		require.NoError(t, err)
	}
	s := &orchestrationruntime.Service{Repo: repo, Runs: runs}
	for range 2 {
		run, err := runs.ClaimNextEligibleRun(ctx)
		require.NoError(t, err)
		handled, err := s.Process(ctx, run)
		if run.AgentProfileID == "office-worker" {
			require.NoError(t, err)
			require.False(t, handled, "a run of an unregistered persona stays with Office")
			continue
		}
		require.True(t, handled, "a registered coordinator's run is never handed to Office")
		require.ErrorIs(t, err, orchestrationruntime.ErrOrchestrationDisabled)
	}
}

func TestCoordinatorFeatureGateNativeConversationDispatch(t *testing.T) {
	_, tasks, repo, taskID := coordinatorConversationFixture(t)
	task, err := tasks.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	for _, runtime := range []*orchestrationruntime.Service{nil, {Repo: repo}} {
		guard := coordinatorDispatchGuard(runtime, repo)
		require.ErrorIs(t, guard(context.Background(), dispatchTarget(task.ID, nil)), orchestrationruntime.ErrOrchestrationDisabled)
	}
	loads := 0
	ordinary := executor.DispatchTarget{TaskID: "ordinary-task", Session: func() (*taskmodels.TaskSession, error) {
		loads++
		return nil, nil
	}}
	require.NoError(t, coordinatorDispatchGuard(nil, repo)(context.Background(), ordinary),
		"tasks that are not coordinator conversations keep native dispatch")
	require.Zero(t, loads, "an ordinary dispatch never loads its session")
}

func TestCoordinatorDispatchGuardRequiresClaimedRun(t *testing.T) {
	a, tasks, repo, taskID := coordinatorConversationFixture(t)
	ctx := context.Background()
	db := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	profiles, _, err := settingsstore.Provide(db, db, log)
	require.NoError(t, err)
	runs := runstore.NewWithDB(db, db)
	require.NoError(t, runs.Migrate())
	s := &orchestrationruntime.Service{Enabled: true, Repo: repo, Personas: &personas.Service{Profiles: profiles, Repo: repo}, Runs: runs, Queue: runservice.New(runs, nil, log, nil)}
	task, err := tasks.GetTask(ctx, taskID)
	require.NoError(t, err)
	session := &taskmodels.TaskSession{ID: "session", TaskID: taskID, Metadata: map[string]interface{}{mcpprofile.BrokerPolicyMetadataKey: string(mcpprofile.SurfaceOrchestratorBroker)}}
	guard := coordinatorDispatchGuard(s, repo)
	require.ErrorIs(t, guard(ctx, dispatchTarget(task.ID, session)), orchestrationmodels.ErrConflict, "a native launch outside a claimed run is refused")

	require.NoError(t, s.QueueTurn(ctx, "fixture-chief", taskID, "task_comment", "guard-run", nil))
	run, err := runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, runs.UpdateRunRuntimeSnapshot(ctx, run.ID, "workspace_coordinator", run.Payload, session.ID))
	require.NoError(t, guard(ctx, dispatchTarget(task.ID, session)), "the claimed run's broker session dispatches")
}

func dispatchTarget(taskID string, session *taskmodels.TaskSession) executor.DispatchTarget {
	return executor.DispatchTarget{TaskID: taskID, Session: func() (*taskmodels.TaskSession, error) { return session, nil }}
}
