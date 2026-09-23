package runtime

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
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
