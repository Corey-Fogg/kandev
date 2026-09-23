package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/config"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestDelegatedTaskStoresItsGoalAndSummariesReadIt(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Delivery"})
	require.NoError(t, err)
	goal, err := shared.NewTaskGoal([]string{"Tests pass", "Docs updated"}, time.Now())
	require.NoError(t, err)
	id, err := adapter.CreateWorkspaceTask(ctx, shared.WorkspaceTaskSpec{WorkspaceID: "ws-1", WorkflowID: wf.ID, Title: "With goal", ChiefID: "chief", Goal: goal})
	require.NoError(t, err)
	task, err := svc.GetTask(ctx, id)
	require.NoError(t, err)
	stored := shared.TaskGoalFromMetadata(task.Metadata)
	require.NotNil(t, stored)
	require.Equal(t, goal.Criteria, stored.Criteria)

	writer := orchestrationTaskMetadata{repo: adapter.taskRepo, tasks: svc}
	stall := shared.TaskStall{Outcome: "no_progress", StalledFor: "5m0s", DetectedAt: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)}
	changed, err := writer.SetTaskMetadata(ctx, id, shared.MetaTaskStall, stall)
	require.NoError(t, err)
	require.True(t, changed)
	task, err = svc.GetTask(ctx, id)
	require.NoError(t, err)

	summary := workspaceTaskSummary(task)
	require.Equal(t, &shared.CriteriaProgress{Met: 0, Total: 2}, summary.Criteria)
	require.Equal(t, &stall, summary.Stall)
	detail := workspaceTaskDetailSummary(task)
	require.Equal(t, stored, detail["acceptance_criteria"])
	require.Equal(t, &stall, detail["stall"])

	plain := workspaceTaskSummary(&models.Task{ID: "plain"})
	require.Nil(t, plain.Criteria)
	require.Nil(t, plain.Stall)
}

func TestOrchestrationMetadataWriterOwnsOnlyItsKeys(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	require.NoError(t, adapter.taskRepo.CreateTask(ctx, &models.Task{ID: "delivery", WorkspaceID: "ws-1", Title: "Delivery"}))
	writer := orchestrationTaskMetadata{repo: adapter.taskRepo, tasks: svc}
	_, err := writer.SetTaskMetadata(ctx, "delivery", "orchestration_chief_id", "someone")
	require.Error(t, err, "the runtime can never rewrite delegation ownership")
	changed, err := writer.SetTaskMetadata(ctx, "missing", shared.MetaTaskGoal, nil)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestOrchestrationRoutesAreAbsentWhenTheFeatureIsOff(t *testing.T) {
	router := gin.New()
	registerOrchestration(routeParams{router: router, features: config.FeaturesConfig{Orchestration: false}})
	for _, path := range []string{
		"/api/v1/orchestration/workspaces/ws/orchestrators/chief/metrics",
		"/api/v1/orchestration/workspaces/ws/orchestrators/chief/proposals",
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNotFound, w.Code, path)
	}
}
