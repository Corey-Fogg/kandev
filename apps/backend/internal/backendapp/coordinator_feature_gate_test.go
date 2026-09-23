package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	officestore "github.com/kandev/kandev/internal/office/repository/sqlite"
	orchestrationmodels "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestration/personas"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	runstore "github.com/kandev/kandev/internal/runs/repository/sqlite"
	runservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-ORCHESTRATION-ASSISTANT-010.1, AC-ORCHESTRATION-ASSISTANT-010.3
func TestOrchestratorFeatureGateNativeRunMatrix(t *testing.T) {
	a, _, _, _ := coordinatorConversationFixture(t)
	db := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	office, err := officestore.NewWithDB(db, db, nil)
	require.NoError(t, err)
	for _, orchestration := range []bool{false, true} {
		t.Run(fmt.Sprintf("orchestration=%v", orchestration), func(t *testing.T) {
			var features config.FeaturesConfig
			require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"orchestration":%v}`, orchestration)), &features))
			allowed, err := orchestrationRunGuard(features, office)(context.Background(), "fixture-chief")
			require.NoError(t, err)
			require.Equal(t, orchestration, allowed)
		})
	}
}

func TestCoordinatorFeatureGateNativeConversationDispatch(t *testing.T) {
	_, tasks, repo, taskID := coordinatorConversationFixture(t)
	task, err := tasks.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	for _, runtime := range []*orchestrationruntime.Service{nil, {Repo: repo}} {
		guard := coordinatorDispatchGuard(runtime, repo)
		require.ErrorIs(t, guard(context.Background(), task, nil, "profile"), orchestrationruntime.ErrOrchestrationDisabled)
	}
	ordinary := &taskmodels.Task{ID: "ordinary-task", WorkspaceID: task.WorkspaceID}
	require.NoError(t, coordinatorDispatchGuard(nil, repo)(context.Background(), ordinary, nil, "profile"),
		"tasks that are not coordinator conversations keep native dispatch")
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
	require.ErrorIs(t, guard(ctx, task, session, "profile"), orchestrationmodels.ErrConflict, "a native launch outside a claimed run is refused")

	require.NoError(t, s.QueueTurn(ctx, "fixture-chief", taskID, "task_comment", "guard-run", nil))
	run, err := runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, runs.UpdateRunRuntimeSnapshot(ctx, run.ID, "workspace_coordinator", run.Payload, session.ID))
	require.NoError(t, guard(ctx, task, session, "profile"), "the claimed run's broker session dispatches")
}
