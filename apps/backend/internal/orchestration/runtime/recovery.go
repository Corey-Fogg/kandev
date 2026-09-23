package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"

	runmodels "github.com/kandev/kandev/internal/runs/models"
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
	if err != nil || run.AgentProfileID != owner {
		c.AbortWithStatus(404)
		return
	}
	if run.Status != statusFailed && run.Status != "cancelled" {
		fail(c, fmt.Errorf("conversation run is not retryable"))
		return
	}
	var payload map[string]any
	if err = json.Unmarshal([]byte(run.Payload), &payload); err != nil {
		fail(c, err)
		return
	}
	if payload["task_id"] != c.Param("id") {
		c.AbortWithStatus(404)
		return
	}
	if err = h.Service.QueueTurn(c.Request.Context(), owner, c.Param("id"), run.Reason, "retry:"+run.ID, payload); err != nil {
		fail(c, err)
		return
	}
	c.JSON(202, gin.H{"ok": true})
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
