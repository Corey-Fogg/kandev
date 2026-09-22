package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestDeliveryListingExcludesRoutineAndOfficeTasksWithoutReclassifyingRoutines(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	routineWorkflow, err := repo.EnsureRoutineWorkflow(ctx, "ws-1")
	require.NoError(t, err)
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "delivery", WorkspaceID: "ws-1", Name: "Delivery"}))
	now := time.Now().UTC()
	for _, task := range []*models.Task{
		{ID: "routine-task", WorkflowID: routineWorkflow, Title: "Routine sweep"},
		{ID: "project-task", WorkflowID: "delivery", Title: "Office project task", ProjectID: "project-1"},
		{ID: "delivery-task", WorkflowID: "delivery", Title: "Ordinary delivery"},
	} {
		task.WorkspaceID, task.State, task.CreatedAt, task.UpdatedAt = "ws-1", "TODO", now, now
		require.NoError(t, repo.CreateTask(ctx, task))
	}

	routine, err := repo.GetTask(ctx, "routine-task")
	require.NoError(t, err)
	require.False(t, routine.IsFromOffice, "routine tasks remain Kanban tasks outside the delivery overview")

	rows, total, err := repo.ListDeliveryTasksByWorkspace(ctx, "ws-1", 1, 10, "updated_at_desc")
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, rows, 1)
	require.Equal(t, "delivery-task", rows[0].ID)
}
