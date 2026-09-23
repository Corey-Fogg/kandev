package backendapp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceOrchestratorNativeTaskLifecycle(t *testing.T) {
	a, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Delivery"})
	require.NoError(t, err)
	for i, id := range []string{"backlog", "progress", "done"} {
		require.NoError(t, a.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: id, WorkflowID: wf.ID, Name: id, Position: i, AllowManualMove: true}))
	}
	id, err := a.CreateWorkspaceTask(ctx, shared.WorkspaceTaskSpec{WorkspaceID: "ws-1", ChiefID: "chief", WorkflowID: wf.ID, Title: "Synthetic task", Description: "Initial description", ExecutionMode: "execute"})
	require.NoError(t, err)
	command := func(body string) error {
		var command shared.WorkspaceTaskCommand
		require.NoError(t, json.Unmarshal([]byte(body), &command))
		command.WorkspaceID, command.ChiefID, command.TaskID = "ws-1", "chief", id
		return a.ManageWorkspaceTask(ctx, command)
	}
	require.NoError(t, command(`{"action":"edit","title":"Updated synthetic task","description":"Revised description","priority":"high"}`))
	task, err := svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "Updated synthetic task", task.Title)
	require.Equal(t, "Revised description", task.Description)
	require.Equal(t, "high", task.Priority)
	require.NoError(t, command(`{"action":"move","workflow_step_id":"progress"}`))
	task, err = svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "progress", task.WorkflowStepID)
	require.NoError(t, command(`{"action":"archive"}`))
	task, err = svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, task.ArchivedAt)
	require.NoError(t, command(`{"action":"delete"}`))
	_, err = svc.GetTask(ctx, id)
	require.Error(t, err)
}

func TestWorkspaceTaskMutationsRejectForeignAndConversationTasks(t *testing.T) {
	a, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	for _, task := range []*models.Task{
		{ID: "foreign-control", WorkspaceID: "other", Title: "Foreign"},
		{ID: "chat-control", WorkspaceID: "ws-1", Title: "Conversation", IsEphemeral: true},
	} {
		require.NoError(t, a.taskRepo.CreateTask(ctx, task))
		for _, action := range []string{"edit", "move", "archive", "delete"} {
			require.Error(t, a.ManageWorkspaceTask(ctx, shared.WorkspaceTaskCommand{WorkspaceID: "ws-1", TaskID: task.ID, Action: action}))
		}
		_, err := a.taskRepo.GetTask(ctx, task.ID)
		require.NoError(t, err)
	}
}

func TestWorkspaceMoveHonorsReviewAndTargetPolicy(t *testing.T) {
	a, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Delivery"})
	require.NoError(t, err)
	for i, id := range []string{"review", "done", "automatic"} {
		require.NoError(t, a.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: id, WorkflowID: wf.ID, Name: id, Position: i, AllowManualMove: id != "automatic"}))
	}
	task := &models.Task{ID: "review-control", WorkspaceID: "ws-1", WorkflowID: wf.ID, WorkflowStepID: "review", Title: "Synthetic review", State: "REVIEW"}
	require.NoError(t, a.taskRepo.CreateTask(ctx, task))
	participant := &wfmodels.WorkflowStepParticipant{StepID: "review", Role: wfmodels.ParticipantRoleReviewer, AgentProfileID: "reviewer", DecisionRequired: true}
	require.NoError(t, a.workflow.UpsertStepParticipant(ctx, participant))
	cmd := shared.WorkspaceTaskCommand{WorkspaceID: "ws-1", TaskID: task.ID, Action: "move", WorkflowStepID: "done"}
	require.ErrorContains(t, a.ManageWorkspaceTask(ctx, cmd), "pending")
	current, err := svc.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "review", current.WorkflowStepID)
	require.NoError(t, a.workflow.RecordStepDecision(ctx, &wfmodels.WorkflowStepDecision{TaskID: task.ID, StepID: "review", ParticipantID: participant.ID, Decision: "approved"}))
	cmd.WorkflowStepID = "automatic"
	require.ErrorContains(t, a.ManageWorkspaceTask(ctx, cmd), "manual moves")
	cmd.WorkflowStepID = "done"
	require.NoError(t, a.ManageWorkspaceTask(ctx, cmd))
}

func TestWorkspaceMoveIntoACompletingStepRequiresMetCriteria(t *testing.T) {
	a, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Delivery"})
	require.NoError(t, err)
	require.NoError(t, a.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "progress", WorkflowID: wf.ID, Name: "progress", Position: 0, AllowManualMove: true}))
	require.NoError(t, a.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "done", WorkflowID: wf.ID, Name: "Done", Position: 1, AllowManualMove: true, CompleteTaskOnEnter: true}))
	goal, err := shared.NewTaskGoal([]string{"Tests pass"}, time.Now())
	require.NoError(t, err)
	task := &models.Task{ID: "gated", WorkspaceID: "ws-1", WorkflowID: wf.ID, WorkflowStepID: "progress", Title: "Gated", State: "IN_PROGRESS",
		Metadata: map[string]interface{}{shared.MetaTaskGoal: goal}}
	require.NoError(t, a.taskRepo.CreateTask(ctx, task))
	cmd := shared.WorkspaceTaskCommand{WorkspaceID: "ws-1", TaskID: task.ID, Action: "move", WorkflowStepID: "done"}
	err = a.ManageWorkspaceTask(ctx, cmd)
	require.ErrorIs(t, err, shared.ErrCriteriaUnmet)
	require.ErrorContains(t, err, "c1")
	current, err := svc.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "progress", current.WorkflowStepID)
	met := true
	require.NoError(t, goal.Verify([]shared.CriterionVerification{{ID: "c1", Met: &met, Evidence: "go test passed"}}, "run", time.Now()))
	_, err = a.taskRepo.SetTaskMetadataKeyIfNotArchived(ctx, task.ID, shared.MetaTaskGoal, goal)
	require.NoError(t, err)
	require.NoError(t, a.ManageWorkspaceTask(ctx, cmd))
}
