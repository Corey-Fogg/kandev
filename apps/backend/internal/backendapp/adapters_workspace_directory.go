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

// catalogWorkflow and catalogStep are the compact catalog rows.
type catalogWorkflow struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	AgentProfileID string `json:"agent_profile_id,omitempty"`
}

type catalogStep struct {
	ID              string `json:"id"`
	WorkflowID      string `json:"workflow_id"`
	Name            string `json:"name"`
	Position        int    `json:"position"`
	IsStartStep     bool   `json:"is_start_step"`
	AllowManualMove bool   `json:"allow_manual_move"`
	AgentProfileID  string `json:"agent_profile_id,omitempty"`
}

func compactWorkflows(workflows []*models.Workflow) []catalogWorkflow {
	rows := []catalogWorkflow{}
	for _, workflow := range workflows {
		if workflow == nil || workflow.Hidden {
			continue
		}
		rows = append(rows, catalogWorkflow{ID: workflow.ID, Name: workflow.Name, Description: workspaceExportText(workflow.Description, 300), AgentProfileID: workflow.AgentProfileID})
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

func compactSteps(steps []*workflowmodels.WorkflowStep) []catalogStep {
	rows := []catalogStep{}
	for _, step := range sortedSteps(steps) {
		rows = append(rows, catalogStep{ID: step.ID, WorkflowID: step.WorkflowID, Name: step.Name, Position: step.Position, IsStartStep: step.IsStartStep, AllowManualMove: step.AllowManualMove, AgentProfileID: step.AgentProfileID})
	}
	return rows
}
