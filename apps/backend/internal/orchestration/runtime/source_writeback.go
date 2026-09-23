package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
	store "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	sourceWriteBackTimeout  = 60 * time.Second
	sourceWriteBackErrorMax = 300
	integrationUnavailable  = "integration is unavailable"
)

// observeSourceWriteBack comments on, and optionally moves, a delegated
// task's source issue when the task reaches review or completes. The
// orchestrator's settings choose what is written; the ledger makes each
// transition write at most once.
func (s *Service) observeSourceWriteBack(ctx context.Context, taskID string) error {
	task, agentID, conversation, err := s.delegatingCoordinator(ctx, taskID)
	if err != nil || agentID == "" || models.TaskSourceIssue(task.Metadata) == nil {
		return err
	}
	settings, active, err := s.activeSettings(ctx, agentID)
	if err != nil || !active {
		return err
	}
	state := string(task.State)
	wantComment := settings.AutoCommentSource && (state == stateReview || state == stateCompleted)
	wantMove := settings.AutoMoveSourceDone && state == stateCompleted
	episode, claimed, err := s.Repo.ObserveTaskState(ctx, task.ID, state, wantComment || wantMove)
	if err != nil || !claimed {
		return err
	}
	if s.SourceIssues == nil {
		return s.Repo.FinishSourceWriteBack(ctx, task.ID, episode, store.WriteBackSkipped, false, "", "tracker write-back is unavailable")
	}
	var update models.SourceIssueUpdate
	if wantComment {
		update.Comment = s.sourceWriteBackComment(ctx, task, state)
	}
	if wantMove {
		update.State = models.SourceStateDone
	}
	detached := context.WithoutCancel(ctx)
	s.runWriteBack(func() { s.writeBack(detached, task, agentID, conversation, episode, update) })
	return nil
}

// activeSettings returns a registered orchestrator's settings and whether it
// is running. A paused or stopped orchestrator records no task state.
func (s *Service) activeSettings(ctx context.Context, agentID string) (models.OrchestratorSettings, bool, error) {
	a, err := s.Repo.OrchestratorAssignment(ctx, agentID)
	if err != nil || a == nil {
		return models.OrchestratorSettings{}, false, err
	}
	persona, err := s.Personas.GetAgentInstance(ctx, agentID)
	if err != nil {
		return models.OrchestratorSettings{}, false, err
	}
	return a.OrchestratorSettings, !paused(persona), nil
}

func (s *Service) runWriteBack(job func()) {
	if s.WriteBackRunner != nil {
		s.WriteBackRunner(job)
		return
	}
	go job()
}

// writeBack performs one claimed write-back and records its outcome. A
// failure other than a missing integration wakes the coordinator once.
func (s *Service) writeBack(ctx context.Context, task *taskmodels.Task, agentID, conversation string, episode int64, update models.SourceIssueUpdate) {
	ctx, cancel := context.WithTimeout(ctx, sourceWriteBackTimeout)
	defer cancel()
	result, err := s.SourceIssues.UpdateSourceIssue(ctx, task.WorkspaceID, task.ID, update)
	status, errText := store.WriteBackPosted, ""
	switch {
	case err == nil:
	case strings.Contains(err.Error(), integrationUnavailable):
		status, errText = store.WriteBackSkipped, clip(err.Error(), sourceWriteBackErrorMax)
	default:
		status, errText = store.WriteBackFailed, clip(redaction.NewRedactor().String(err.Error()), sourceWriteBackErrorMax)
	}
	if finishErr := s.Repo.FinishSourceWriteBack(ctx, task.ID, episode, status, result.Commented, result.State, errText); finishErr != nil {
		logger.Default().Warn("orchestration: recording a source issue write-back failed", zap.String("task_id", task.ID), zap.Error(finishErr))
	}
	if status != store.WriteBackFailed {
		return
	}
	if err := s.reportWriteBackFailure(ctx, task, agentID, conversation, episode, errText); err != nil {
		logger.Default().Warn("orchestration: reporting a failed source issue write-back failed", zap.String("task_id", task.ID), zap.Error(err))
	}
}

func (s *Service) reportWriteBackFailure(ctx context.Context, task *taskmodels.Task, agentID, conversation string, episode int64, errText string) error {
	update, _, err := s.describeTask(ctx, task)
	if err != nil {
		return err
	}
	update.SourceWriteBackError = errText
	key := fmt.Sprintf("source-writeback:%s:%s:%d", agentID, task.ID, episode)
	return s.QueueTurn(ctx, agentID, conversation, callbackReason, key, map[string]any{callbacksKey: []taskUpdate{update}})
}

// sourceWriteBackComment is the tracker comment for a task that reached
// review or completed.
func (s *Service) sourceWriteBackComment(ctx context.Context, task *taskmodels.Task, state string) string {
	var text strings.Builder
	if state == stateCompleted {
		fmt.Fprintf(&text, "Kandev: \"%s\" is complete.", task.Title)
	} else {
		fmt.Fprintf(&text, "Kandev: \"%s\" is ready for review.", task.Title)
	}
	if s.PullRequests != nil {
		prs, err := s.PullRequests(ctx, []string{task.ID})
		if pr, ok := prs[task.ID]; err == nil && ok && pr.URL != "" {
			fmt.Fprintf(&text, "\nPull request: %s", pr.URL)
		}
	}
	if goal := models.TaskGoalFromMetadata(task.Metadata); goal != nil {
		progress := goal.Progress()
		fmt.Fprintf(&text, "\nAcceptance criteria: %d of %d met.", progress.Met, progress.Total)
	}
	comment := redaction.NewRedactor().String(text.String())
	if len(comment) > models.SourceCommentMaxBytes {
		comment = clip(comment, models.SourceCommentMaxBytes-len("\n[Excerpt]"))
	}
	return comment
}
