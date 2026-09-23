package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"

	runmodels "github.com/kandev/kandev/internal/runs/models"
	runservice "github.com/kandev/kandev/internal/runs/service"
)

func (h *Handler) retry(c *gin.Context) {
	owner, _, ok := h.scopedConversation(c)
	if !ok {
		return
	}
	if _, ok := c.Get("agent_claims"); ok {
		c.AbortWithStatus(403)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		RunID     string `json:"run_id"`
		Action    string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	if req.Action != "resume" && req.Action != "fresh_start" {
		fail(c, fmt.Errorf("invalid recovery action"))
		return
	}
	run, err := h.retryTarget(c.Request.Context(), req.RunID, req.SessionID)
	if err == nil {
		run, err = h.latestRetry(c.Request.Context(), run)
	}
	if err != nil || run.AgentProfileID != owner {
		c.AbortWithStatus(404)
		return
	}
	h.requeue(c, owner, run)
}

// requeue queues a retry of run, or reports that one is already queued.
func (h *Handler) requeue(c *gin.Context, owner string, run *runmodels.Run) {
	if run.Status == runQueued || run.Status == statusClaimed {
		c.JSON(200, gin.H{"ok": true, retryStatusKey: retryAlreadyQueued})
		return
	}
	if run.Status != statusFailed && run.Status != "cancelled" {
		fail(c, fmt.Errorf("conversation run is not retryable"))
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Payload), &payload); err != nil {
		fail(c, err)
		return
	}
	if payload["task_id"] != c.Param("id") {
		c.AbortWithStatus(404)
		return
	}
	outcome, err := h.Service.queueTurn(c.Request.Context(), owner, c.Param("id"), run.Reason, retryKeyPrefix+run.ID, payload)
	if err != nil {
		fail(c, err)
		return
	}
	if outcome == runservice.QueueOutcomeDeduped {
		c.JSON(200, gin.H{"ok": true, retryStatusKey: retryAlreadyQueued})
		return
	}
	c.JSON(202, gin.H{"ok": true, retryStatusKey: retryQueued})
}

const (
	retryKeyPrefix     = "retry:"
	retryStatusKey     = "status"
	retryQueued        = "queued"
	retryAlreadyQueued = "already_queued"
	runQueued          = "queued"
	// maxRetryChain bounds how many earlier retries are followed.
	maxRetryChain = 50
)

// latestRetry follows a run's retries to the newest one, so retrying a turn
// whose earlier retry failed retries that retry instead of deduplicating
// against it.
func (h *Handler) latestRetry(ctx context.Context, run *runmodels.Run) (*runmodels.Run, error) {
	for i := 0; i < maxRetryChain; i++ {
		next, err := h.Service.Repo.RunByIdempotencyKey(ctx, retryKeyPrefix+run.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return run, nil
		}
		if err != nil {
			return nil, err
		}
		run = next
	}
	return run, nil
}

// retryTarget selects the run to retry: the named run, or the latest run of
// the named session. A turn that never bound a session is retried by run id.
func (h *Handler) retryTarget(ctx context.Context, runID, sessionID string) (*runmodels.Run, error) {
	if runID != "" {
		return h.Service.Runs.GetRunByID(ctx, runID)
	}
	if sessionID == "" {
		return nil, fmt.Errorf("run_id or session_id is required")
	}
	return h.Service.Runs.LatestRunForSession(ctx, sessionID)
}

// RecoverInterrupted runs before subscriptions and dispatch start. An interrupted
// conversation must be explicitly retried, never replayed after an unknown external write.
func (s *Service) RecoverInterrupted(ctx context.Context) error {
	rows, err := s.Repo.InterruptedRuns(ctx)
	if err != nil {
		return err
	}
	for _, run := range rows {
		if err := s.Runs.RecordFailure(ctx, run.ID, "Conversation interrupted by backend restart. Inspect the latest task results before retrying."); err != nil {
			return err
		}
		if _, err := s.Runs.FinishRun(ctx, run.ID, statusFailed, nil); err != nil {
			return err
		}
		if err := s.Repo.SetRuntimeWorking(ctx, run.AgentProfileID, false); err != nil {
			return err
		}
	}
	if s.Queue != nil {
		return s.DispatchIntake(ctx)
	}
	return nil
}
