package models

// WorkspaceTaskSpec selects existing Kanban resources for delegated work.
// WorkspaceID is supplied by the authenticated runtime, never by the agent payload.
type WorkspaceTaskSpec struct {
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
	// Source records the tracker issue the task implements; it sets the
	// issue metadata and ExternalID.
	Source *SourceIssue
	// Goal is the task's acceptance criteria, built by the runtime; the
	// adapter only stores it.
	Goal *TaskGoal
}

// WorkspaceTaskCommand manages an existing delivery task under a signed workspace.
type WorkspaceTaskCommand struct {
	SessionID      string `json:"session_id"`
	Prompt         string `json:"prompt"`
	Mode           string `json:"mode,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	PendingID      string `json:"pending_id,omitempty"`
	OptionID       string `json:"option_id,omitempty"`
	WorkspaceID    string
	ChiefID        string
	TaskID         string
	Action         string                    `json:"action"`
	AssigneeID     string                    `json:"assignee"`
	Title          *string                   `json:"title,omitempty"`
	Description    *string                   `json:"description,omitempty"`
	Priority       *string                   `json:"priority,omitempty"`
	ParentID       *string                   `json:"parent_id,omitempty"`
	WorkflowID     string                    `json:"workflow_id,omitempty"`
	WorkflowStepID string                    `json:"workflow_step_id,omitempty"`
	Position       *int                      `json:"position,omitempty"`
	Answers        []WorkspaceQuestionAnswer `json:"answers,omitempty"`
	Rejected       bool                      `json:"rejected,omitempty"`
	RejectReason   string                    `json:"reject_reason,omitempty"`
	// AcceptanceCriteria replaces the task's criteria (set_criteria).
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	// Criteria records checked criteria (verify_criteria).
	Criteria []CriterionVerification `json:"criteria,omitempty"`
}

// WorkspaceQuestionAnswer answers one question of a delegated task's pending
// clarification bundle.
type WorkspaceQuestionAnswer struct {
	QuestionID      string   `json:"question_id"`
	SelectedOptions []string `json:"selected_options,omitempty"`
	CustomText      string   `json:"custom_text,omitempty"`
}
