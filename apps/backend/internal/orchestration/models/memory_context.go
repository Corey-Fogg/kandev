package models

// ContextScope names the resources a memory entry may be scoped to.
type ContextScope struct {
	ProfileID     string `json:"profile_id"`
	ProjectID     string `json:"project_id,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	TaskID        string `json:"task_id,omitempty"`
}

type ContextMemory struct {
	ID              string `json:"id"`
	Revision        int64  `json:"revision"`
	Scope           string `json:"scope"`
	ScopeID         string `json:"scope_id"`
	SourceCommentID string `json:"source_comment_id,omitempty"`
	Confirmed       bool   `json:"confirmed"`
	Content         string `json:"content"`
	Truncated       bool   `json:"truncated,omitempty"`
}

func (m AgentMemory) MatchesContext(b *AssistantBinding, s ContextScope) bool {
	if m.ForgottenAt != nil {
		return false
	}
	if m.OwnerUserID != "" && m.OwnerUserID != b.OwnerUserID {
		return false
	}
	switch m.Scope {
	case "user":
		return m.OwnerUserID == b.OwnerUserID && m.ScopeID == b.OwnerUserID
	case "workspace":
		return m.ScopeID == "" || m.ScopeID == b.WorkspaceID
	case "project":
		return s.ProjectID != "" && m.ScopeID == s.ProjectID
	case "environment":
		return s.EnvironmentID != "" && m.ScopeID == s.EnvironmentID
	case "task":
		return s.TaskID != "" && m.ScopeID == s.TaskID
	default:
		return false
	}
}
