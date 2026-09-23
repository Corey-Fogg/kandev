package models

// WorkspaceTaskSpec selects existing Kanban resources for delegated work.
// WorkspaceID is supplied by the authenticated runtime, never by the agent payload.
type WorkspaceTaskSpec struct {
	DirectProfile  bool
	WorkspaceID    string
	WorkflowID     string
	WorkflowStepID string
	ExecutionMode  string
	RepositoryID   string
	ParentID       string
	ChiefID        string
	AssigneeID     string
	Title          string
	Description    string
	ExternalID     string
}

// WorkspaceTaskCommand manages an existing delivery task under a signed workspace.
type WorkspaceTaskCommand struct {
	DirectProfile  bool   `json:"-"`
	SessionID      string `json:"session_id"`
	Prompt         string `json:"prompt"`
	Mode           string `json:"mode,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	PendingID      string `json:"pending_id,omitempty"`
	OptionID       string `json:"option_id,omitempty"`
	WorkspaceID    string
	ChiefID        string
	TaskID         string
	Action         string  `json:"action"`
	AssigneeID     string  `json:"assignee"`
	Title          *string `json:"title,omitempty"`
	Description    *string `json:"description,omitempty"`
	Priority       *string `json:"priority,omitempty"`
	ParentID       *string `json:"parent_id,omitempty"`
	WorkflowID     string  `json:"workflow_id,omitempty"`
	WorkflowStepID string  `json:"workflow_step_id,omitempty"`
	Position       *int    `json:"position,omitempty"`
}
