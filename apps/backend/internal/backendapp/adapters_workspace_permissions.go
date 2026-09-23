package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/clarification"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
)

// workspacePendingQuestion is the coordinator's view of one pending
// clarification bundle on a delegated task.
type workspacePendingQuestion struct {
	SessionID string                          `json:"session_id"`
	PendingID string                          `json:"pending_id"`
	Questions []workspacePendingQuestionEntry `json:"questions"`
}

type workspacePendingQuestionEntry struct {
	ID      string                           `json:"question_id"`
	Title   string                           `json:"title,omitempty"`
	Prompt  string                           `json:"prompt"`
	Options []workspacePendingQuestionOption `json:"options,omitempty"`
}

type workspacePendingQuestionOption struct {
	ID          string `json:"option_id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

func (a *taskCreatorAdapter) WorkspaceTaskPermissions(ctx context.Context, workspaceID, taskID, sessionID string) (any, error) {
	task, err := a.taskSvc.GetTask(ctx, taskID)
	if err != nil || task.WorkspaceID != workspaceID || task.IsEphemeral || task.IsFromOffice {
		return nil, fmt.Errorf("delivery task unavailable")
	}
	if a.orch == nil {
		return nil, fmt.Errorf("orchestrator unavailable")
	}
	permissions, err := a.orch.ListPendingAgentPermissions(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	questions, err := a.pendingWorkspaceQuestions(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"permissions": permissions, "questions": questions}, nil
}

// pendingWorkspaceQuestions lists the task's open clarification bundles,
// optionally narrowed to one session.
func (a *taskCreatorAdapter) pendingWorkspaceQuestions(ctx context.Context, taskID, sessionID string) ([]workspacePendingQuestion, error) {
	filter := models.PendingInteractionFilter{TaskIDs: []string{taskID}, Kinds: []string{string(models.InteractionKindClarification)}}
	if sessionID != "" {
		filter.SessionIDs = []string{sessionID}
	}
	pending, err := a.taskSvc.ListPendingInteractions(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := make([]workspacePendingQuestion, 0, len(pending))
	for _, interaction := range pending {
		if interaction.TaskID != taskID || interaction.Kind != models.InteractionKindClarification {
			continue
		}
		row := workspacePendingQuestion{SessionID: interaction.SessionID, PendingID: interaction.ID}
		for _, question := range interaction.Questions {
			entry := workspacePendingQuestionEntry{ID: question.ID, Title: workspaceExportText(question.Title, 200), Prompt: workspaceExportText(question.Prompt, 2000)}
			for _, option := range question.Options {
				entry.Options = append(entry.Options, workspacePendingQuestionOption{ID: option.ID, Label: workspaceExportText(option.Label, 300), Description: workspaceExportText(option.Description, 600)})
			}
			row.Questions = append(row.Questions, entry)
		}
		result = append(result, row)
	}
	return result, nil
}

func (a *taskCreatorAdapter) controlWorkspacePermission(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	if !delegatedTo(task, command.ChiefID) {
		return fmt.Errorf("%s is limited to tasks delegated to this Orchestrator", command.Action)
	}
	if a.orch == nil {
		return fmt.Errorf("orchestrator unavailable")
	}
	if command.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if command.Action == workspaceSessionMode {
		return a.orch.SetTaskSessionPermissionMode(ctx, task.ID, command.SessionID, command.Mode)
	}
	permissions, err := a.orch.ListPendingAgentPermissions(ctx, task.ID, command.SessionID)
	if err != nil {
		return err
	}
	if err := validateWorkspacePermissionChoice(permissions, command); err != nil {
		return err
	}
	_, err = a.orch.ResolveAgentPermission(ctx, orchestrator.ResolveAgentPermissionRequest{
		TaskID: task.ID, SessionID: command.SessionID, RequestID: command.RequestID,
		PendingID: command.PendingID, OptionID: command.OptionID, Source: models.PermissionSourceAutomation,
	})
	return err
}

// delegatedTo reports whether the task was created or adopted by the calling
// coordinator. Only those tasks accept its permission and question relay.
func delegatedTo(task *models.Task, chiefID string) bool {
	chief, _ := task.Metadata["orchestration_chief_id"].(string)
	return chief != "" && chief == chiefID
}

// answerWorkspaceQuestion settles a pending clarification bundle of a task
// delegated to the calling coordinator through the shared native resolver.
func (a *taskCreatorAdapter) answerWorkspaceQuestion(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	if a.clarifications == nil {
		return fmt.Errorf("clarification resolver unavailable")
	}
	if !delegatedTo(task, command.ChiefID) {
		return fmt.Errorf("answer_question is limited to tasks delegated to this Orchestrator")
	}
	if command.SessionID == "" || command.PendingID == "" {
		return fmt.Errorf("session_id and pending_id are required")
	}
	pending, err := a.pendingWorkspaceQuestions(ctx, task.ID, command.SessionID)
	if err != nil {
		return err
	}
	if !pendingQuestionListed(pending, command.SessionID, command.PendingID) {
		return fmt.Errorf("select a pending question from task_permissions")
	}
	outcome := clarification.Outcome{Rejected: command.Rejected, RejectReason: command.RejectReason}
	for _, answer := range command.Answers {
		outcome.Answers = append(outcome.Answers, clarification.Answer{QuestionID: answer.QuestionID, SelectedOptions: answer.SelectedOptions, CustomText: answer.CustomText})
	}
	_, claimed, err := a.clarifications.ResolveBundle(ctx, command.PendingID, outcome)
	if err != nil {
		return err
	}
	if !claimed {
		return fmt.Errorf("question was already answered")
	}
	return nil
}

func pendingQuestionListed(pending []workspacePendingQuestion, sessionID, pendingID string) bool {
	for _, row := range pending {
		if row.SessionID == sessionID && row.PendingID == pendingID {
			return true
		}
	}
	return false
}

func validateWorkspacePermissionChoice(permissions []streams.PendingAgentPermission, command shared.WorkspaceTaskCommand) error {
	for _, permission := range permissions {
		if permission.SessionID != command.SessionID || permission.RequestID != command.RequestID || permission.PendingID != command.PendingID {
			continue
		}
		for _, option := range permission.Options {
			if option.OptionID == command.OptionID && (option.Kind == streams.PermissionOptionKindAllowOnce || option.Kind == streams.PermissionOptionKindRejectOnce) {
				return nil
			}
		}
	}
	return fmt.Errorf("select an exact live allow_once or reject_once option from task_permissions; persistent grants are unavailable")
}
