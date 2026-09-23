package backendapp

import (
	"context"
	"errors"
	"fmt"
	"maps"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// CreateWorkspaceTask keeps delivery on the workspace's existing workflow engine.
func (a *taskCreatorAdapter) CreateWorkspaceTask(ctx context.Context, spec shared.WorkspaceTaskSpec) (string, error) {
	workflowID, err := a.workspaceDeliveryWorkflow(ctx, spec.WorkspaceID, spec.WorkflowID)
	if err != nil {
		return "", err
	}
	if err := a.validateWorkspaceEntry(ctx, workflowID, spec); err != nil {
		return "", err
	}
	metadata := map[string]interface{}{"orchestration_chief_id": spec.ChiefID, "orchestration_managed": true}
	if spec.Source != nil {
		maps.Copy(metadata, spec.Source.Metadata())
		spec.ExternalID = spec.Source.ExternalID()
	}
	profileID, err := a.directWorkerProfile(ctx, spec, metadata)
	if err != nil {
		return "", err
	}
	req := &taskservice.CreateTaskRequest{
		PlanMode:    spec.ExecutionMode == "design",
		WorkspaceID: spec.WorkspaceID, WorkflowID: workflowID, WorkflowStepID: spec.WorkflowStepID,
		Title: spec.Title, Description: spec.Description, ParentID: spec.ParentID,
		AssigneeAgentProfileID: profileID, Origin: models.TaskOriginAgentCreated, Metadata: metadata, ExternalID: spec.ExternalID,
	}
	if spec.RepositoryID != "" {
		repository, err := a.taskSvc.GetRepository(ctx, spec.RepositoryID)
		if err != nil || repository.WorkspaceID != spec.WorkspaceID {
			return "", fmt.Errorf("repository must belong to the workspace")
		}
		req.Repositories = []taskservice.TaskRepositoryInput{{RepositoryID: spec.RepositoryID, BaseBranch: repository.DefaultBranch}}
	}
	result, err := a.taskSvc.CreateTask(ctx, req)
	if err != nil {
		return "", err
	}
	if result.Outcome != taskservice.CreateTaskOutcomeCreated {
		return "", &shared.DuplicateTaskError{TaskID: result.Task.ID, Archived: result.Task.ArchivedAt != nil}
	}
	if _, _, err := a.taskSvc.SettleExternalID(ctx, result.Task.ID, result.Task.ExternalID); err != nil {
		return "", err
	}
	return result.Task.ID, nil
}

// validateWorkspaceEntry admits an explicit entry step only where the
// selected workflow permits a task to start or be moved manually.
func (a *taskCreatorAdapter) validateWorkspaceEntry(ctx context.Context, workflowID string, spec shared.WorkspaceTaskSpec) error {
	if spec.ExecutionMode != "" && spec.ExecutionMode != "execute" && spec.ExecutionMode != "design" {
		return fmt.Errorf("execution_mode must be design or execute")
	}
	if spec.WorkflowStepID == "" {
		return nil
	}
	if a.workflow == nil {
		return fmt.Errorf("workflow entry validation unavailable")
	}
	step, err := a.workflow.GetStep(ctx, spec.WorkflowStepID)
	if err != nil || step.WorkflowID != workflowID {
		return fmt.Errorf("entry step must belong to the selected workflow")
	}
	if !step.IsStartStep && !step.AllowManualMove {
		return fmt.Errorf("workflow policy does not permit entry at this step")
	}
	return nil
}

func (a *taskCreatorAdapter) workspaceDeliveryWorkflow(ctx context.Context, workspaceID, selected string) (string, error) {
	workflows, err := a.taskSvc.ListWorkflows(ctx, workspaceID, false)
	if err != nil {
		return "", err
	}
	var eligible []*models.Workflow
	for _, workflow := range workflows {
		if workflow.Hidden {
			continue
		}
		if selected == workflow.ID {
			return selected, nil
		}
		eligible = append(eligible, workflow)
	}
	if selected != "" {
		return "", fmt.Errorf("select a delivery workflow in this workspace")
	}
	if len(eligible) != 1 {
		return "", fmt.Errorf("select workflow_id explicitly: workspace has %d delivery workflows", len(eligible))
	}
	return eligible[0].ID, nil
}

// WorkspaceCatalog lists the workspace's delivery configuration. The compact
// form carries only identifying fields; full adds complete repository and
// step configuration for workspace administration.
func (a *taskCreatorAdapter) WorkspaceCatalog(ctx context.Context, workspaceID string, full bool) (any, error) {
	workflows, err := a.taskSvc.ListWorkflows(ctx, workspaceID, false)
	if err != nil {
		return nil, err
	}
	repositories, err := a.taskSvc.ListRepositories(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	var steps []*workflowmodels.WorkflowStep
	if a.workflow != nil {
		if steps, err = a.workflow.ListStepsByWorkspaceID(ctx, workspaceID); err != nil {
			return nil, err
		}
	}
	if full {
		result := map[string]any{workspaceWorkflowsKey: workflows, workspaceRepositoriesKey: repositories}
		if a.workflow != nil {
			result["workflow_steps"] = steps
		}
		return result, nil
	}
	return map[string]any{workspaceWorkflowsKey: compactWorkflows(workflows), workspaceRepositoriesKey: compactRepositories(repositories), "workflow_steps": compactSteps(steps)}, nil
}

func (a *taskCreatorAdapter) ManageWorkspaceTask(ctx context.Context, command shared.WorkspaceTaskCommand) error {
	task, err := a.taskSvc.GetTask(ctx, command.TaskID)
	if err != nil || task.WorkspaceID != command.WorkspaceID {
		return fmt.Errorf("task must belong to this workspace")
	}
	if task.IsFromOffice || task.IsEphemeral {
		return fmt.Errorf("select a Kanban delivery task")
	}
	return a.dispatchWorkspaceTask(ctx, task, command)
}

func (a *taskCreatorAdapter) dispatchWorkspaceTask(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	switch command.Action {
	case "session_mode", "resolve_permission":
		return a.controlWorkspacePermission(ctx, task, command)
	case "answer_question":
		return a.answerWorkspaceQuestion(ctx, task, command)
	case "message":
		return a.messageWorkspaceTask(ctx, task, command)
	case "adopt", "assign":
		return a.adoptWorkspaceTask(ctx, task, command)
	case "start":
		return a.startAssignedWorkspaceTask(ctx, task)
	case "repair_session":
		return a.repairWorkspaceSession(ctx, task, command)
	case "edit":
		return a.editWorkspaceTask(ctx, task, command)
	case "move":
		return a.moveWorkspaceTask(ctx, task, command)
	case "archive":
		return a.taskSvc.ArchiveTask(ctx, task.ID)
	case "delete":
		return a.taskSvc.DeleteTask(ctx, task.ID)
	case "stop":
		if a.orch == nil {
			return fmt.Errorf("orchestrator unavailable")
		}
		return a.orch.StopTask(ctx, task.ID, "coordinator requested stop", false)
	default:
		return fmt.Errorf("unsupported task action")
	}
}

func (a *taskCreatorAdapter) requireIdleWorkspaceTask(ctx context.Context, taskID string) error {
	if a.taskRepo == nil {
		return nil
	}
	session, err := a.taskRepo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil && !errors.Is(err, models.ErrTaskSessionNotFound) {
		return err
	}
	if session != nil {
		return fmt.Errorf("stop active execution before changing task ownership")
	}
	return nil
}

func (a *taskCreatorAdapter) adoptWorkspaceTask(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	if command.AssigneeID != "" {
		return a.assignDirectWorkspaceTask(ctx, task, command)
	}
	if command.Action == "adopt" && a.taskRepo != nil {
		if _, err := a.taskRepo.SetTaskMetadataKeyIfNotArchived(ctx, task.ID, "orchestration_managed", true); err != nil {
			return err
		}
		return a.observeWorkspaceTask(ctx, task.ID, command.ChiefID)
	}

	if err := a.requireIdleWorkspaceTask(ctx, task.ID); err != nil {
		return err
	}
	metadata := maps.Clone(task.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["orchestration_chief_id"] = command.ChiefID
	_, err := a.taskSvc.UpdateTask(ctx, task.ID, &taskservice.UpdateTaskRequest{Metadata: metadata})
	return err
}

func (a *taskCreatorAdapter) observeWorkspaceTask(ctx context.Context, taskID, chiefID string) error {
	changed, err := a.taskRepo.SetTaskMetadataKeyIfNotArchived(ctx, taskID, "orchestration_chief_id", chiefID)
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("task is archived or unavailable")
	}
	task, err := a.taskSvc.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	a.taskSvc.PublishTaskUpdated(ctx, task)
	return nil
}
