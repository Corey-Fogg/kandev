package sqlite

import (
	"context"
	"slices"

	"github.com/kandev/kandev/internal/office/models"
)

// ExternalAgents lists agent profiles that a runtime outside Office owns.
// Office never lists, schedules or handles events for those profiles.
type ExternalAgents interface {
	RegisteredProfileIDs(ctx context.Context) ([]string, error)
}

// SetExternalAgents installs the owner of agent profiles that Office must
// leave alone. It is wired once at startup, before the repository is used.
func (r *Repository) SetExternalAgents(external ExternalAgents) { r.externalAgents = external }

// IsExternalAgentTask reports whether the task's runner is an agent profile
// owned outside Office. An unreadable task is treated as Office-owned.
func (r *Repository) IsExternalAgentTask(ctx context.Context, taskID string) (bool, error) {
	if r.externalAgents == nil {
		return false, nil
	}
	ids, err := r.externalAgents.RegisteredProfileIDs(ctx)
	if err != nil || len(ids) == 0 {
		return false, err
	}
	fields, err := r.GetTaskExecutionFields(ctx, taskID)
	if err != nil || fields.AssigneeAgentProfileID == "" {
		return false, nil
	}
	return slices.Contains(ids, fields.AssigneeAgentProfileID), nil
}

func (r *Repository) withoutExternalAgents(ctx context.Context, agents []*models.AgentInstance) ([]*models.AgentInstance, error) {
	if r.externalAgents == nil || len(agents) == 0 {
		return agents, nil
	}
	ids, err := r.externalAgents.RegisteredProfileIDs(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(agents, func(agent *models.AgentInstance) bool {
		return slices.Contains(ids, agent.ID)
	}), nil
}
