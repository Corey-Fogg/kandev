package models

type WorkspaceTaskSummary struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspace_id"`
	Title          string `json:"title"`
	State          string `json:"state"`
	WorkflowID     string `json:"workflow_id"`
	WorkflowStepID string `json:"workflow_step_id"`
}
