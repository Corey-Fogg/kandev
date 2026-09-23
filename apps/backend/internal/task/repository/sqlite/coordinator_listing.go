package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) ListKanbanTasksByWorkspace(ctx context.Context, workspaceID string, q models.KanbanTaskQuery) ([]*models.Task, int, error) {
	return r.listWorkspaceTasks(ctx, workspaceID, q.WorkflowID, q.RepositoryID, q.Query, q.Page, q.PageSize, q.Sort, q.IncludeArchived, false, false, true, q.OnlyArchived, true, false)
}

const kanbanWorkflowFilter = ` AND workflow_id IN (
 SELECT w.id FROM workflows w JOIN workspaces ws ON ws.id = w.workspace_id
 WHERE w.hidden = 0 AND (ws.office_workflow_id IS NULL OR w.id != ws.office_workflow_id))`
