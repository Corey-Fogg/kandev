package runtime

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

const (
	duplicateKey = "duplicate"
	archivedKey  = "archived"
)

// updateSourceIssue comments on or moves the tracker issue a workspace task
// records in its metadata. The issue is never taken from the request.
func (h *Handler) updateSourceIssue(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	if h.Service.SourceIssues == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{errorResponseKey: "tracker write-back is unavailable"})
		return
	}
	var req models.SourceIssueUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	req.Comment = strings.TrimSpace(req.Comment)
	if req.Comment == "" && req.State == "" {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "comment or state is required"})
		return
	}
	if len(req.Comment) > models.SourceCommentMaxBytes {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "comment must be 4000 bytes or fewer"})
		return
	}
	switch req.State {
	case "", models.SourceStateStarted, models.SourceStateReview, models.SourceStateDone:
	default:
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "state must be started, review or done"})
		return
	}
	req.Comment = redaction.NewRedactor().String(req.Comment)
	task, err := h.Service.Tasks.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil || task.WorkspaceID != claims.WorkspaceID {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	result, err := h.Service.SourceIssues.UpdateSourceIssue(c.Request.Context(), claims.WorkspaceID, task.ID, req)
	var stateErr *models.SourceStateError
	switch {
	case errors.As(err, &stateErr):
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: err.Error(), "available": stateErr.Available, "commented": result.Commented})
	case errors.Is(err, models.ErrNoSourceIssue):
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: err.Error()})
	case err != nil:
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{errorResponseKey: err.Error(), "commented": result.Commented})
	default:
		c.JSON(http.StatusOK, result)
	}
}
