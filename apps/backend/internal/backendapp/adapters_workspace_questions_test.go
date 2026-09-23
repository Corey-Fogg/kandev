package backendapp

import (
	"context"
	"testing"
	"time"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func seedDelegatedQuestion(t *testing.T, adapter *taskCreatorAdapter, chief string) *models.Task {
	t.Helper()
	ctx := context.Background()
	at := time.Now().UTC()
	task := &models.Task{ID: "delegated", WorkspaceID: "ws-1", Title: "Delegated work", State: "REVIEW", Metadata: map[string]any{"orchestration_chief_id": chief}}
	require.NoError(t, adapter.taskRepo.CreateTask(ctx, task))
	require.NoError(t, adapter.taskRepo.CreateTaskSession(ctx, &models.TaskSession{ID: "worker", TaskID: task.ID, State: models.TaskSessionStateWaitingForInput}))
	require.NoError(t, adapter.taskRepo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskSessionID: "worker", TaskID: task.ID, StartedAt: at, CreatedAt: at, UpdatedAt: at}))
	require.NoError(t, adapter.taskRepo.CreateMessage(ctx, &models.Message{
		ID: "question", TaskSessionID: "worker", TaskID: task.ID, TurnID: "turn", AuthorType: models.MessageAuthorAgent,
		Content: "Which base branch?", Type: models.MessageTypeClarificationRequest, RequestsInput: true,
		Metadata: map[string]any{"pending_id": "pending", "question_id": "q1", "status": "pending", "question": map[string]any{
			"id": "q1", "prompt": "Which base branch?", "options": []any{map[string]any{"option_id": "develop", "label": "develop"}},
		}},
		CreatedAt: at, UpdatedAt: at,
	}))
	return task
}

func TestWorkspaceTaskPermissionsListsPendingQuestions(t *testing.T) {
	adapter, _ := newOfficeTaskAdapterHarness(t)
	seedDelegatedQuestion(t, adapter, "chief")
	questions, err := adapter.pendingWorkspaceQuestions(context.Background(), "delegated", "")
	require.NoError(t, err)
	require.Len(t, questions, 1)
	require.Equal(t, "worker", questions[0].SessionID)
	require.Equal(t, "pending", questions[0].PendingID)
	require.Len(t, questions[0].Questions, 1)
	require.Equal(t, "q1", questions[0].Questions[0].ID)
	require.Equal(t, "Which base branch?", questions[0].Questions[0].Prompt)
}

func TestAnswerWorkspaceQuestionResolvesOnlyDelegatedPendingBundle(t *testing.T) {
	adapter, _ := newOfficeTaskAdapterHarness(t)
	task := seedDelegatedQuestion(t, adapter, "chief")
	resolver := &recordingBundleResolver{claimed: true}
	adapter.clarifications = resolver
	ctx := context.Background()
	answer := shared.WorkspaceTaskCommand{Action: "answer_question", ChiefID: "chief", SessionID: "worker", PendingID: "pending",
		Answers: []shared.WorkspaceQuestionAnswer{{QuestionID: "q1", SelectedOptions: []string{"develop"}}}}

	foreign := answer
	foreign.ChiefID = "other-chief"
	require.Error(t, adapter.answerWorkspaceQuestion(ctx, task, foreign), "a task delegated elsewhere is refused")
	unknown := answer
	unknown.PendingID = "invented"
	require.Error(t, adapter.answerWorkspaceQuestion(ctx, task, unknown), "only a listed pending bundle is answerable")
	require.Empty(t, resolver.pendingID)

	require.NoError(t, adapter.answerWorkspaceQuestion(ctx, task, answer))
	require.Equal(t, "pending", resolver.pendingID)
	require.Len(t, resolver.outcome.Answers, 1)
	require.Equal(t, []string{"develop"}, resolver.outcome.Answers[0].SelectedOptions)

	resolver.claimed = false
	require.Error(t, adapter.answerWorkspaceQuestion(ctx, task, answer), "a lost claim is reported")
}

func TestWorkspacePermissionRelayIsLimitedToDelegatedTasks(t *testing.T) {
	adapter, _ := newOfficeTaskAdapterHarness(t)
	task := seedDelegatedQuestion(t, adapter, "chief")
	for _, action := range []string{"session_mode", "resolve_permission"} {
		command := shared.WorkspaceTaskCommand{Action: action, ChiefID: "other-chief", SessionID: "worker", Mode: "auto", RequestID: "request", PendingID: "pending", OptionID: "allow"}
		err := adapter.controlWorkspacePermission(context.Background(), task, command)
		require.ErrorContains(t, err, "limited to tasks delegated to this Orchestrator", action)
	}
}
