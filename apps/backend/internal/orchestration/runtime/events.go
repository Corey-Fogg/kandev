package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestration/models"
	runmodels "github.com/kandev/kandev/internal/runs/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func decode(event *bus.Event) (map[string]any, error) {
	raw, err := json.Marshal(event.Data)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	err = json.Unmarshal(raw, &data)
	return data, err
}
func (s *Service) Subscribe(eb bus.EventBus) (func(), error) {
	ids := []bus.Subscription{}
	cleanup := func() {
		for _, id := range ids {
			_ = id.Unsubscribe()
		}
	}
	for _, subject := range []string{events.TaskStateChanged, events.TaskMoved, events.SessionPendingActionChanged, events.AgentStalled, events.TaskStalled, events.AgentTurnMessageSaved, events.AgentCompleted, events.AgentStopped, events.AgentFailed} {
		id, err := eb.Subscribe(subject, s.onEvent)
		if err != nil {
			cleanup()
			return nil, err
		}
		ids = append(ids, id)
	}
	return cleanup, nil
}
func (s *Service) onEvent(ctx context.Context, event *bus.Event) error {
	data, err := decode(event)
	if err != nil {
		return err
	}
	taskID := s.eventTask(ctx, data)
	if taskID == "" {
		return nil
	}
	switch event.Type {
	case events.AgentStalled, events.TaskStalled:
		return s.stallCallback(ctx, event.Type, taskID, data)
	case events.TaskStateChanged, events.TaskMoved:
		return s.taskCallback(ctx, taskID)
	case events.SessionPendingActionChanged:
		// A delegated session that starts waiting on a permission or question
		// wakes its coordinator even when the task state does not change.
		if action, _ := data["pending_action"].(string); action == "" {
			return nil
		}
		return s.taskCallback(ctx, taskID)
	}
	owner, _, err := s.Repo.ConversationOwner(ctx, taskID)
	if err != nil || owner == "" {
		return nil
	}
	if event.Type == events.AgentTurnMessageSaved {
		return s.bridgeReply(ctx, event, data, taskID, owner)
	}

	// The execution owner decides recovery before publishing terminal UI state.
	if event.Type == events.AgentFailed && s.FailureHandlerInstalled {
		return nil
	}
	return s.finishTurn(ctx, event, data, taskID, owner)
}

func (s *Service) finishTurn(ctx context.Context, event *bus.Event, data map[string]any, taskID, owner string) error {
	switch event.Type {
	case events.AgentCompleted, events.AgentStopped, events.AgentFailed:
	default:
		return nil
	}
	run, err := s.Runs.GetClaimedRunByTaskID(ctx, taskID)
	if err != nil || run == nil {
		return nil
	}
	if !matchesClaimedTurn(event, data, run) || s.isRetiredExecution(data) {
		return nil
	}
	status := "finished"
	if event.Type == events.AgentFailed {
		if retried, err := s.recordTurnFailure(ctx, run, data); retried || err != nil {
			return err
		}
		status = statusFailed
	}
	finished, err := s.Runs.FinishRun(ctx, run.ID, status, nil)
	if err != nil {
		return err
	}
	s.retiredExecutions.Delete(run.ID)
	if err := s.Repo.SetRuntimeWorking(ctx, owner, false); err != nil {
		return err
	}
	if finished == nil || status != statusFailed {
		return nil
	}
	message, _ := data["error_message"].(string)
	if message == "" {
		message = "the agent failed"
	}
	return s.postTurnFailure(ctx, finished, message)
}

// recordTurnFailure retries a failed turn when it can; otherwise it records
// the failure on the run. It reports true when the turn was retried.
func (s *Service) recordTurnFailure(ctx context.Context, run *runmodels.Run, data map[string]any) (bool, error) {
	if retried, err := s.retryTurn(ctx, run, data); retried || err != nil {
		return true, err
	}
	message, _ := data["error_message"].(string)
	return false, s.Runs.RecordFailure(ctx, run.ID, message)
}
func matchesClaimedTurn(event *bus.Event, data map[string]any, run *runmodels.Run) bool {
	sessionID, _ := data["session_id"].(string)
	eventRun, _ := data["run_id"].(string)
	if eventRun != run.ID || run.SessionID == "" || sessionID != run.SessionID {
		return false
	}
	return run.ClaimedAt == nil || event.Timestamp.IsZero() || !event.Timestamp.Before(*run.ClaimedAt)
}

func (s *Service) bridgeReply(ctx context.Context, event *bus.Event, data map[string]any, taskID, owner string) error {
	var err error

	body, _ := data["agent_text"].(string)
	turnID, _ := data["turn_id"].(string)
	sessionID, _ := data["session_id"].(string)
	if body == "" && turnID != "" {
		body, err = s.Tasks.GetLastAgentMessageForTurn(ctx, turnID)
	}
	if body == "" && turnID == "" && sessionID != "" {
		body, err = s.lastAgentMessage(ctx, sessionID)
	}
	if err != nil {
		return err
	}
	if body == "" {
		return nil
	}
	session, _ := data["session_id"].(string)
	identity := event.ID + session
	if turnID != "" {
		identity = turnID + session
	}
	if run, e := s.Runs.GetClaimedRunByTaskID(ctx, taskID); turnID == "" && e == nil && run != nil {
		identity = run.ID + session
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:%x", identity, sha256.Sum256([]byte(body))))).String()
	return s.Repo.PutComment(ctx, &models.TaskComment{ID: id, TaskID: taskID, AuthorID: owner, AuthorType: authorTypeAgent, Body: body, Source: "session"})

}

func (s *Service) eventTask(ctx context.Context, data map[string]any) string {
	if task, _ := data[taskIDKey].(string); task != "" {
		return task
	}
	session, _ := data["session_id"].(string)
	reader, ok := s.Tasks.(interface {
		GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error)
	})
	if !ok || session == "" {
		return ""
	}
	row, err := reader.GetTaskSession(ctx, session)
	if err != nil || row == nil {
		return ""
	}
	return row.TaskID
}
