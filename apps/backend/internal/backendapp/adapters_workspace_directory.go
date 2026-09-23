package backendapp

import (
	"context"
	"sort"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// WorkspaceDirectory lists the delivery workflows with their steps and the
// repositories, carrying only the IDs and names a coordinator selects from.
func (a *taskCreatorAdapter) WorkspaceDirectory(ctx context.Context, workspaceID string) (shared.WorkspaceDirectory, error) {
	directory := shared.WorkspaceDirectory{Workflows: []shared.DirectoryWorkflow{}, Repositories: []shared.DirectoryRepository{}}
	workflows, err := a.taskSvc.ListWorkflows(ctx, workspaceID, false)
	if err != nil {
		return directory, err
	}
	repositories, err := a.taskSvc.ListRepositories(ctx, workspaceID)
	if err != nil {
		return directory, err
	}
	var steps []*workflowmodels.WorkflowStep
	if a.workflow != nil {
		if steps, err = a.workflow.ListStepsByWorkspaceID(ctx, workspaceID); err != nil {
			return directory, err
		}
	}
	byWorkflow := map[string][]shared.DirectoryStep{}
	for _, step := range sortedSteps(steps) {
		byWorkflow[step.WorkflowID] = append(byWorkflow[step.WorkflowID], shared.DirectoryStep{ID: step.ID, Name: step.Name, Start: step.IsStartStep, ManualEntry: step.AllowManualMove})
	}
	for _, workflow := range workflows {
		if workflow == nil || workflow.Hidden {
			continue
		}
		directory.Workflows = append(directory.Workflows, shared.DirectoryWorkflow{ID: workflow.ID, Name: workflow.Name, Steps: byWorkflow[workflow.ID]})
	}
	directory.Repositories = compactRepositories(repositories)
	return directory, nil
}

func sortedSteps(steps []*workflowmodels.WorkflowStep) []*workflowmodels.WorkflowStep {
	ordered := make([]*workflowmodels.WorkflowStep, 0, len(steps))
	for _, step := range steps {
		if step != nil {
			ordered = append(ordered, step)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].WorkflowID != ordered[j].WorkflowID {
			return ordered[i].WorkflowID < ordered[j].WorkflowID
		}
		return ordered[i].Position < ordered[j].Position
	})
	return ordered
}

func compactWorkflows(workflows []*models.Workflow) []map[string]any {
	rows := []map[string]any{}
	for _, workflow := range workflows {
		if workflow == nil || workflow.Hidden {
			continue
		}
		rows = append(rows, map[string]any{"id": workflow.ID, "name": workflow.Name, "description": workspaceExportText(workflow.Description, 300), "agent_profile_id": workflow.AgentProfileID})
	}
	return rows
}

func compactRepositories(repositories []*models.Repository) []shared.DirectoryRepository {
	rows := []shared.DirectoryRepository{}
	for _, repository := range repositories {
		if repository != nil {
			rows = append(rows, shared.DirectoryRepository{ID: repository.ID, Name: repository.Name, DefaultBranch: repository.DefaultBranch, RemoteURL: repository.RemoteURL})
		}
	}
	return rows
}

func compactSteps(steps []*workflowmodels.WorkflowStep) []map[string]any {
	rows := []map[string]any{}
	for _, step := range sortedSteps(steps) {
		rows = append(rows, map[string]any{"id": step.ID, "workflow_id": step.WorkflowID, "name": step.Name, "position": step.Position, "is_start_step": step.IsStartStep, "allow_manual_move": step.AllowManualMove, "agent_profile_id": step.AgentProfileID})
	}
	return rows
}
