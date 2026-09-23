package orchestration

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestration/models"
)

// metrics reports an orchestrator's delegated outcomes over 7 or 30 days.
func (h *Handler) metrics(c *gin.Context) {
	a := h.runtimeScoped(c)
	if a == nil {
		return
	}
	days := 7
	switch c.Query("days") {
	case "", "7":
	case "30":
		days = 30
	default:
		c.JSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "days must be 7 or 30"})
		return
	}
	result, err := h.Runtime.CoordinatorMetrics(c.Request.Context(), c.Param("wsId"), a.ID, days)
	if errors.Is(err, models.ErrProposalNotFound) {
		c.JSON(http.StatusNotFound, gin.H{errorResponseKey: errOrchestratorNotFound})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
