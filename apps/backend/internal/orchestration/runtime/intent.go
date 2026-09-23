package runtime

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	runmodels "github.com/kandev/kandev/internal/runs/models"
)

func (s *Service) withIntentRevision(ctx context.Context, taskID string, payload map[string]any) (map[string]any, error) {
	copy := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		copy[key] = value
	}
	if _, present := copy[intentRevisionKey]; !present {
		revision, err := s.Repo.IntentRevision(ctx, taskID)
		if err != nil {
			return nil, err
		}
		copy[intentRevisionKey] = revision
	}
	return copy, nil
}

// stampLaunchIntent gives a turn that is not a user message the intent
// revision current at launch. Callback, stall, retry and automation turns
// carry no user intent of their own, so a user message that arrived while
// they were queued must not make them fail the dispatch guard. User-message
// turns keep their queue-time revision and are superseded instead.
func (s *Service) stampLaunchIntent(ctx context.Context, run *runmodels.Run, taskID string, payload map[string]any) (map[string]any, error) {
	if run.Reason == commentReason {
		return payload, nil
	}
	revision, err := s.Repo.IntentRevision(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if stamped, ok := payload[intentRevisionKey].(float64); ok && int64(stamped) == revision {
		return payload, nil
	}
	copy := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		copy[key] = value
	}
	copy[intentRevisionKey] = revision
	data, err := json.Marshal(copy)
	if err != nil {
		return nil, err
	}
	if err := s.Repo.SetClaimedRunPayload(ctx, run.ID, string(data)); err != nil {
		return nil, err
	}
	run.Payload = string(data)
	return copy, nil
}

func (h *Handler) currentIntent(c *gin.Context, taskID, payload string) bool {
	var snapshot struct {
		Revision int64 `json:"intent_revision"`
	}
	if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
		c.AbortWithStatus(http.StatusForbidden)
		return false
	}
	revision, err := h.Service.Repo.IntentRevision(c.Request.Context(), taskID)
	if err != nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return false
	}
	if snapshot.Revision != revision {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{errorResponseKey: "intent_superseded", intentRevisionKey: revision})
		return false
	}
	return true
}
