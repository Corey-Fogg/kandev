package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	runmodels "github.com/kandev/kandev/internal/runs/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	callbackReason       = "workspace_task_callback"
	commentReason        = "task_comment"
	callbacksKey         = "callbacks"
	legacyCallbackKey    = "callback"
	callbackExcerptBytes = 600
	callbackQuestionMax  = 3
	supersededOutcome    = "superseded"
)

// taskUpdate is the digest a delegated task contributes to a coordinator
// turn: enough state to report or relay without another read.
type taskUpdate struct {
	TaskID             string   `json:"task_id"`
	Title              string   `json:"title"`
	State              string   `json:"state"`
	SessionID          string   `json:"session_id,omitempty"`
	SessionState       string   `json:"session_state,omitempty"`
	LastMessage        string   `json:"last_message_excerpt,omitempty"`
	Error              string   `json:"error,omitempty"`
	PendingPermissions int      `json:"pending_permissions,omitempty"`
	PendingQuestions   int      `json:"pending_questions,omitempty"`
	Questions          []string `json:"questions,omitempty"`
}

// actionable reports whether the update warrants a coordinator turn: the
// task reached a reportable state or a delegated session waits on input.
func (u taskUpdate) actionable() bool {
	switch u.State {
	case "REVIEW", "COMPLETED", "FAILED", "WAITING_FOR_INPUT", "BLOCKED":
		return true
	}
	return u.PendingPermissions+u.PendingQuestions > 0
}

// taskCallback wakes the delegating coordinator with the task's digest. An
// update whose digest was already queued for that coordinator is suppressed.
func (s *Service) taskCallback(ctx context.Context, taskID string) error {
	task, err := s.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	id, _ := task.Metadata["orchestration_chief_id"].(string)
	if id == "" {
		return nil
	}
	a, err := s.Personas.GetAgentInstance(ctx, id)
	if err != nil || a.WorkspaceID != task.WorkspaceID {
		return nil
	}
	conversation, err := s.Repo.EnsureAgentConversation(ctx, a)
	if err != nil {
		return err
	}
	if conversation.TaskID == taskID {
		return nil
	}
	update, digest, err := s.describeTask(ctx, task)
	if err != nil || !update.actionable() {
		return err
	}
	key := fmt.Sprintf("workspace-task-callback:%s:%s:%s", id, taskID, digest)
	return s.QueueTurn(ctx, id, conversation.TaskID, callbackReason, key, map[string]any{callbacksKey: []taskUpdate{update}})
}

// describeTask builds the task's update and a digest of every field that
// makes an update new: state, latest session, full latest reply, error and
// the identities of pending permissions and questions.
func (s *Service) describeTask(ctx context.Context, task *taskmodels.Task) (taskUpdate, string, error) {
	update := taskUpdate{TaskID: task.ID, Title: clip(task.Title, 300), State: string(task.State)}
	identity := []string{update.State}
	if launch, ok := taskmodels.LoadTaskLaunchError(task.Metadata); ok {
		update.Error = clip(launch.Code+": "+launch.Message, 300)
		identity = append(identity, "launch:"+launch.Stamp())
	}
	sessions, err := s.Tasks.ListTaskSessions(ctx, task.ID)
	if err != nil {
		return update, "", err
	}
	if session := latestSession(sessions); session != nil {
		update.SessionID, update.SessionState = session.ID, string(session.State)
		identity = append(identity, "session:"+session.ID+":"+update.SessionState)
		if agentErr, ok := taskmodels.LoadLastAgentError(session.Metadata); ok && !agentErr.IsDismissed() {
			update.Error = clip(agentErr.Code+": "+agentErr.Message, 300)
			identity = append(identity, "error:"+agentErr.Stamp())
		}
		text, err := s.Tasks.GetLastAgentMessage(ctx, session.ID)
		if err != nil {
			return update, "", err
		}
		update.LastMessage = clip(text, callbackExcerptBytes)
		identity = append(identity, fmt.Sprintf("reply:%x", sha256.Sum256([]byte(text))))
	}
	pending, err := s.Tasks.ListPendingInteractions(ctx, taskmodels.PendingInteractionFilter{TaskIDs: []string{task.ID}})
	if err != nil {
		return update, "", err
	}
	for _, interaction := range pending {
		identity = append(identity, "pending:"+interaction.SessionID+":"+interaction.ID+":"+interaction.RequestID)
		if interaction.Kind == taskmodels.InteractionKindPermission {
			update.PendingPermissions++
			continue
		}
		update.PendingQuestions++
		for _, question := range interaction.Questions {
			if len(update.Questions) < callbackQuestionMax {
				update.Questions = append(update.Questions, clip(question.Prompt, 300))
			}
		}
	}
	return update, fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(identity, "\x00")))), nil
}

func latestSession(sessions []*taskmodels.TaskSession) *taskmodels.TaskSession {
	var latest *taskmodels.TaskSession
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if latest == nil || session.UpdatedAt.After(latest.UpdatedAt) || (session.UpdatedAt.Equal(latest.UpdatedAt) && session.ID > latest.ID) {
			latest = session
		}
	}
	return latest
}

// payloadUpdates reads a run payload's task updates, oldest first.
func payloadUpdates(payload map[string]any) []taskUpdate {
	var updates []taskUpdate
	if raw, ok := payload[callbacksKey]; ok {
		data, _ := json.Marshal(raw)
		_ = json.Unmarshal(data, &updates)
	}
	if raw, ok := payload[legacyCallbackKey]; ok {
		var update taskUpdate
		data, _ := json.Marshal(raw)
		if json.Unmarshal(data, &update) == nil && update.TaskID != "" {
			updates = append(updates, update)
		}
	}
	return updates
}

// mergeUpdates keeps one update per task, the latest one, in first-seen order.
func mergeUpdates(batches ...[]taskUpdate) []taskUpdate {
	index := map[string]int{}
	merged := []taskUpdate{}
	for _, batch := range batches {
		for _, update := range batch {
			if i, seen := index[update.TaskID]; seen {
				merged[i] = update
				continue
			}
			index[update.TaskID] = len(merged)
			merged = append(merged, update)
		}
	}
	return merged
}

// absorbQueuedCallbacks folds the coordinator's queued callback runs into
// this turn, so updates that arrive while the coordinator is busy are
// delivered together in its next turn.
func (s *Service) absorbQueuedCallbacks(ctx context.Context, run *runmodels.Run, payload map[string]any) (map[string]any, error) {
	if run.RetryCount > 0 {
		return payload, nil
	}
	merged := payload
	raw, err := s.Repo.AbsorbQueuedRuns(ctx, run.ID, run.AgentProfileID, callbackReason, "coalesced into run "+run.ID, func(_ string, queued []string) (string, error) {
		batches := [][]taskUpdate{payloadUpdates(payload)}
		for _, row := range queued {
			var other map[string]any
			if err := json.Unmarshal([]byte(row), &other); err != nil {
				return "", err
			}
			batches = append(batches, payloadUpdates(other))
		}
		merged = make(map[string]any, len(payload)+1)
		for key, value := range payload {
			merged[key] = value
		}
		delete(merged, legacyCallbackKey)
		merged[callbacksKey] = mergeUpdates(batches...)
		data, err := json.Marshal(merged)
		return string(data), err
	})
	if err != nil {
		return nil, err
	}
	run.Payload = raw
	return merged, nil
}

// supersededTurn reports whether a user-message turn was overtaken by a newer
// message before it launched. The newer turn's prompt carries both messages.
func (s *Service) supersededTurn(ctx context.Context, run *runmodels.Run, taskID string, payload map[string]any) (bool, error) {
	if run.Reason != commentReason {
		return false, nil
	}
	revision, ok := payload[intentRevisionKey].(float64)
	if !ok {
		return false, nil
	}
	current, err := s.Repo.IntentRevision(ctx, taskID)
	if err != nil {
		return false, err
	}
	return int64(revision) < current, nil
}

// writeTaskUpdates renders a turn's delegated task updates.
func writeTaskUpdates(text *strings.Builder, updates []taskUpdate) {
	if len(updates) == 0 {
		return
	}
	text.WriteString("\nDelegated task updates (captured when each update arrived; read task_details only when this is not enough):\n")
	for _, u := range updates {
		fmt.Fprintf(text, "- %s (task_id=%s, state=%s", u.Title, u.TaskID, u.State)
		if u.SessionID != "" {
			fmt.Fprintf(text, ", session_id=%s, session_state=%s", u.SessionID, u.SessionState)
		}
		text.WriteString(")\n")
		if u.PendingPermissions+u.PendingQuestions > 0 {
			fmt.Fprintf(text, "  Waiting on %d permission request(s) and %d question(s).\n", u.PendingPermissions, u.PendingQuestions)
		}
		for _, question := range u.Questions {
			fmt.Fprintf(text, "  Question: %s\n", question)
		}
		if u.Error != "" {
			fmt.Fprintf(text, "  Error: %s\n", u.Error)
		}
		if u.LastMessage != "" {
			fmt.Fprintf(text, "  Latest reply: %s\n", u.LastMessage)
		}
	}
	text.WriteString("Post only new information in this chat. Review is not completion; do not repeat an answered question or restart work. For a pending permission or question, call task_permissions, then resolve_permission or answer_question only when the user's instructions or memory already settle it; otherwise ask the user here and link the task. If a session stopped on a provider login or OAuth refresh error, call manage_task with action repair_session once, then report the outcome.\n")
}

// updatesForPrompt is the ordered update list a turn renders.
func updatesForPrompt(payload map[string]any) []taskUpdate {
	return mergeUpdates(payloadUpdates(payload))
}
