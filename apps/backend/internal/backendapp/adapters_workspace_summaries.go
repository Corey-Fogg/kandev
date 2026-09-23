package backendapp

import (
	"sort"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
)

func workspaceTaskDetailSummary(task *models.Task) map[string]any {
	return map[string]any{
		"id": task.ID, workspaceKeyWorkspaceID: task.WorkspaceID, workspaceKeyTitle: workspaceExportText(task.Title, 200),
		workspaceKeyDescription: workspaceExportText(task.Description, 1600), "description_truncated": len(task.Description) > 1600,
		workspaceResultStateKey: task.State, "workflow_id": task.WorkflowID, "workflow_step_id": task.WorkflowStepID,
		"parent_id": task.ParentID, "priority": task.Priority,
		"acceptance_criteria": shared.TaskGoalFromMetadata(task.Metadata), "stall": shared.TaskStallFromMetadata(task.Metadata),
	}
}

func workspaceSessionSummaries(sessions []*models.TaskSession) []map[string]any {
	ordered := append([]*models.TaskSession(nil), sessions...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].UpdatedAt.After(ordered[j].UpdatedAt) })
	if len(ordered) > 8 {
		ordered = ordered[:8]
	}
	rows := make([]map[string]any, 0, len(ordered))
	for _, session := range ordered {
		mode, _ := session.Metadata[models.SessionMetaKeySessionMode].(string)
		rows = append(rows, map[string]any{
			"id": session.ID, workspaceKeyAgentProfileID: session.AgentProfileID, workspaceResultStateKey: session.State,
			"error_message": workspaceExportText(session.ErrorMessage, 500), workspaceSessionMode: workspaceExportText(mode, 80),
			"review_status": session.ReviewStatus, sessionUpdatedAtPayloadKey: session.UpdatedAt,
		})
	}
	return rows
}
