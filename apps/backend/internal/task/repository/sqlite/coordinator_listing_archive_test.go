package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestKanbanListingHonoursArchiveFilters(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()
	require.NoError(t, f.repo.CreateTask(ctx, &models.Task{ID: "archived", WorkspaceID: autoOriginWorkspaceID, WorkflowID: autoOriginWorkflowID, Title: "Archived delivery", State: "COMPLETED", Priority: "medium"}))
	require.NoError(t, f.repo.ArchiveTask(ctx, "archived"))
	ids := func(q models.KanbanTaskQuery) []string {
		t.Helper()
		q.Page, q.PageSize = 1, 50
		rows, total, err := f.repo.ListKanbanTasksByWorkspace(ctx, autoOriginWorkspaceID, q)
		require.NoError(t, err)
		require.Equal(t, len(rows), total)
		out := make([]string, 0, len(rows))
		for _, row := range rows {
			out = append(out, row.ID)
		}
		return out
	}
	require.Equal(t, []string{f.boardTaskID}, ids(models.KanbanTaskQuery{}), "archived tasks stay hidden by default")
	require.ElementsMatch(t, []string{f.boardTaskID, "archived"}, ids(models.KanbanTaskQuery{IncludeArchived: true}))
	require.Equal(t, []string{"archived"}, ids(models.KanbanTaskQuery{OnlyArchived: true}))
}
