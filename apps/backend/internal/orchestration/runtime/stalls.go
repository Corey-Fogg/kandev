package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// Stall outcomes a coordinator receives for a delegated task.
const (
	stallNoProgress   = "no_progress"
	stallNeverStarted = "never_started"
	stallOrphaned     = "orphaned"
)

// stallCallback wakes the delegating coordinator when a delegated task stops
// making progress: its agent went silent mid-prompt (no_progress), never
// emitted an event for its prompt (never_started), or its active session has
// no live execution behind it (orphaned). Each stall episode wakes once.
func (s *Service) stallCallback(ctx context.Context, subject, taskID string, data map[string]any) error {
	task, id, conversation, err := s.delegatingCoordinator(ctx, taskID)
	if err != nil || id == "" {
		return err
	}
	outcome, stalledFor, episode := stallDetails(subject, data)
	if outcome == "" {
		return nil
	}
	s.recordStall(ctx, taskID, outcome, stalledFor, data)
	update, _, err := s.describeTask(ctx, task)
	if err != nil {
		return err
	}
	update.StallOutcome, update.StalledFor = outcome, stalledFor
	key := fmt.Sprintf("workspace-task-stall:%s:%s:%s:%s", id, taskID, outcome, episode)
	return s.QueueTurn(ctx, id, conversation, callbackReason, key, map[string]any{callbacksKey: []taskUpdate{update}})
}

// stallDetails classifies a stall event and names its episode: the prompt
// generation of an agent stall, or the sessions and last event of a task stall.
func stallDetails(subject string, data map[string]any) (outcome, stalledFor, episode string) {
	switch subject {
	case events.AgentStalled:
		outcome = stallNoProgress
		if never, _ := data["never_started"].(bool); never {
			outcome = stallNeverStarted
		}
		if nanos, ok := data["stalled_for"].(float64); ok {
			stalledFor = time.Duration(nanos).Round(time.Second).String()
		}
		session, _ := data["session_id"].(string)
		generation, _ := data["prompt_generation"].(float64)
		return outcome, stalledFor, fmt.Sprintf("%s:%.0f", session, generation)
	case events.TaskStalled:
		stalledFor, _ = data["stalled_for"].(string)
		if d, err := time.ParseDuration(stalledFor); err == nil {
			stalledFor = d.Round(time.Second).String()
		}
		last, _ := data["last_event_at"].(string)
		var sessions []string
		if raw, ok := data["session_ids"].([]any); ok {
			for _, value := range raw {
				if id, ok := value.(string); ok {
					sessions = append(sessions, id)
				}
			}
		}
		sort.Strings(sessions)
		return stallOrphaned, stalledFor, strings.Join(sessions, ",") + ":" + last
	}
	return "", "", ""
}

// recordStall stores the stall episode on the task so the Coordinator view
// can show it, even while the coordinator is paused. It is best effort.
func (s *Service) recordStall(ctx context.Context, taskID, outcome, stalledFor string, data map[string]any) {
	if s.TaskMetadata == nil {
		return
	}
	session, _ := data["session_id"].(string)
	stall := models.TaskStall{Outcome: outcome, StalledFor: stalledFor, SessionID: session, DetectedAt: s.now()}
	if _, err := s.TaskMetadata.SetTaskMetadata(ctx, taskID, models.MetaTaskStall, stall); err != nil {
		logger.Default().Warn("orchestration: recording a task stall failed", zap.String("task_id", taskID), zap.Error(err))
	}
}
