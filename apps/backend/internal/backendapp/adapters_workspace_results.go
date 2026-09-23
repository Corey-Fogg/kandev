package backendapp

import (
	"context"
	"errors"
	"unicode/utf8"

	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/redaction"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const (
	workspaceResultStateKey = "state"
	workspaceResultTaskKey  = "task"
	workspaceTasksKey       = "tasks"
	credentialUnavailable   = "unavailable"
	workspaceWorkflowsKey   = "workflows"
)

const (
	workspaceResultContentKey     = "content"
	workspaceResultSortDescending = "desc"
	workspaceRepositoriesKey      = "repositories"
)

// WorkspaceTaskDetails exposes bounded worker results without copying tool transcripts.
func (a *taskCreatorAdapter) WorkspaceTaskDetails(ctx context.Context, workspaceID, taskID string) (any, error) {
	task, err := a.taskSvc.GetTask(ctx, taskID)
	if err != nil || task.WorkspaceID != workspaceID || task.IsEphemeral || task.IsFromOffice {
		return nil, fmt.Errorf("task must belong to this workspace")
	}
	sessions, err := a.taskSvc.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	result := map[string]any{workspaceResultTaskKey: workspaceTaskDetailSummary(task), "sessions": workspaceSessionSummaries(sessions)}
	if err := a.attachWorkspaceSessionResults(ctx, result, sessions); err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return result, nil
	}
	return result, a.attachLatestWorkspaceMessages(ctx, result, sessions)
}

func (a *taskCreatorAdapter) attachLatestWorkspaceMessages(ctx context.Context, result map[string]any, sessions []*models.TaskSession) error {
	latest := sessions[0]
	for _, session := range sessions[1:] {
		if session.UpdatedAt.After(latest.UpdatedAt) {
			latest = session
		}
	}
	messages, more, err := a.taskSvc.ListMessagesPaginated(ctx, taskservice.ListMessagesRequest{TaskSessionID: latest.ID, Limit: 20, Sort: workspaceResultSortDescending, AuthorType: string(models.MessageAuthorAgent)})
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, 6)
	for _, message := range messages {
		if message.Type != "message" && !message.RequestsInput {
			continue
		}
		if len(rows) == 1 {
			more = true
			break
		}
		row := map[string]any{"id": message.ID, workspaceKeyAuthorType: message.AuthorType, workspaceResultContentKey: workspaceExportText(message.Content, 4000), "requests_input": message.RequestsInput, workspaceKeyTruncated: len(message.Content) > 4000}
		rows = append(rows, row)
	}
	result[workspaceKeyMessages], result[workspaceKeyHasMore], result[sessionIDPayloadKey] = rows, more, latest.ID
	return nil
}

func (a *taskCreatorAdapter) attachWorkspaceSessionResults(ctx context.Context, result map[string]any, sessions []*models.TaskSession) error {
	ordered := append([]*models.TaskSession(nil), sessions...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].UpdatedAt.Equal(ordered[j].UpdatedAt) {
			return ordered[i].ID > ordered[j].ID
		}
		return ordered[i].UpdatedAt.After(ordered[j].UpdatedAt)
	})
	result["has_more_sessions"] = len(ordered) > 8
	if len(ordered) > 8 {
		ordered = ordered[:8]
	}
	rows := make([]map[string]any, 0, len(ordered))
	for _, session := range ordered {
		messages, more, err := a.taskSvc.ListMessagesPaginated(ctx, taskservice.ListMessagesRequest{TaskSessionID: session.ID, Limit: 10, Sort: workspaceResultSortDescending, AuthorType: string(models.MessageAuthorAgent)})
		if err != nil {
			return err
		}
		excerpts := make([]map[string]any, 0, 2)
		for _, m := range messages {
			if m.Type != models.MessageTypeMessage {
				continue
			}
			if len(excerpts) == 2 {
				more = true
				break
			}
			excerpts = append(excerpts, map[string]any{"id": m.ID, workspaceResultContentKey: workspaceExportText(m.Content, 256), workspaceKeyAuthorType: m.AuthorType, workspaceKeyTruncated: len(m.Content) > 256})
		}
		rows = append(rows, map[string]any{sessionIDPayloadKey: session.ID, "profile_id": session.AgentProfileID, workspaceResultStateKey: session.State, "review_status": session.ReviewStatus, workspaceKeyMessages: excerpts, workspaceKeyHasMore: more})
	}
	result["session_results"] = rows
	return nil
}

func (a *taskCreatorAdapter) messageWorkspaceTask(ctx context.Context, task *models.Task, command shared.WorkspaceTaskCommand) error {
	if a.orch == nil {
		return fmt.Errorf("orchestrator unavailable")
	}
	if strings.TrimSpace(command.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	sessionID, err := a.messageSessionID(ctx, task.ID, command.SessionID)
	if err != nil {
		return err
	}
	session, err := a.taskSvc.GetTaskSession(ctx, sessionID)
	if err != nil || session.TaskID != task.ID {
		return fmt.Errorf("session must belong to this task")
	}
	if session.State == models.TaskSessionStateRunning {
		_, err = a.orch.SteerTask(ctx, task.ID, session.ID, command.Prompt, "", false, nil)
		if !errors.Is(err, orchestrator.ErrSteerNotEligible) {
			return err
		}
	}
	// Delivery returns at worker acceptance; the worker's reply reaches the
	// coordinator as a task update, never through this call.
	return pluginsTaskMessengerAdapter{tasks: a.taskSvc, orch: a.orch}.promptWithResume(ctx, task.ID, session.ID, command.Prompt)
}

// messageSessionID returns the requested session, or the task's most recently
// updated session when none is named.
func (a *taskCreatorAdapter) messageSessionID(ctx context.Context, taskID, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	sessions, err := a.taskSvc.ListTaskSessions(ctx, taskID)
	if err != nil {
		return "", err
	}
	var latest *models.TaskSession
	for _, session := range sessions {
		if latest == nil || session.UpdatedAt.After(latest.UpdatedAt) {
			latest = session
		}
	}
	if latest == nil {
		return "", fmt.Errorf("task has no session to message; use start")
	}
	return latest.ID, nil
}

// WorkspaceTaskSummaries pages the workspace's delivery tasks, most recently
// updated first.
func (a *taskCreatorAdapter) WorkspaceTaskSummaries(ctx context.Context, workspace string, page, limit int) ([]shared.WorkspaceTaskSummary, bool, error) {
	if err := a.taskSvc.AuthorizeWorkspaceScope(ctx, workspace, authz.ScopeWorkspaceRead); err != nil {
		return nil, false, err
	}
	if page < 1 || page > 100000 || limit < 1 || limit > 100 {
		return nil, false, fmt.Errorf("invalid task page")
	}
	tasks, total, err := a.taskSvc.ListDeliveryTasksByWorkspace(ctx, workspace, page, limit, "updated_at_desc")
	if err != nil {
		return nil, false, err
	}
	rows := []shared.WorkspaceTaskSummary{}
	for _, task := range tasks {
		rows = append(rows, workspaceTaskSummary(task))
	}
	if err := a.enrichTaskSummaries(ctx, rows); err != nil {
		return nil, false, err
	}
	return rows, page*limit < total, nil
}

func workspaceTaskSummary(task *models.Task) shared.WorkspaceTaskSummary {
	summary := shared.WorkspaceTaskSummary{ID: task.ID, WorkspaceID: task.WorkspaceID, Title: workspaceExportText(task.Title, 300), State: string(task.State), WorkflowID: task.WorkflowID, WorkflowStepID: task.WorkflowStepID,
		UpdatedAt: task.UpdatedAt, ParentID: task.ParentID, ExternalID: workspaceExportText(task.ExternalID, 300), Source: shared.TaskSourceIssue(task.Metadata),
		Stall: shared.TaskStallFromMetadata(task.Metadata)}
	if goal := shared.TaskGoalFromMetadata(task.Metadata); goal != nil {
		progress := goal.Progress()
		summary.Criteria = &progress
	}
	return summary
}

func workspaceExportText(value string, limit int) string {
	value = redaction.NewRedactor().String(value)
	if len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}
