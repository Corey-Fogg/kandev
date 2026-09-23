package models

type WorkspaceTaskSummary struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspace_id"`
	Title          string `json:"title"`
	State          string `json:"state"`
	WorkflowID     string `json:"workflow_id"`
	WorkflowStepID string `json:"workflow_step_id"`
}

// WorkspaceDirectory is the compact ID directory a coordinator needs to
// create, assign and move tasks without reading the full configuration.
type WorkspaceDirectory struct {
	Workflows    []DirectoryWorkflow   `json:"workflows"`
	Repositories []DirectoryRepository `json:"repositories"`
}

type DirectoryWorkflow struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Steps []DirectoryStep `json:"steps"`
}

type DirectoryStep struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Start       bool   `json:"is_start_step,omitempty"`
	ManualEntry bool   `json:"allow_manual_move,omitempty"`
}

type DirectoryRepository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch,omitempty"`
	RemoteURL     string `json:"remote_url,omitempty"`
}
