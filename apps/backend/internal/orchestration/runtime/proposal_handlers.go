package runtime

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
)

const proposalIDKey = "proposal_id"

// proposeTask answers create_task with a proposal when the orchestrator asks
// before creating tasks.
func (h *Handler) proposeTask(c *gin.Context, claims *runtimeauth.AgentClaims, spec models.ProposalSpec, truncated bool) {
	p, created, duplicate, err := h.Service.ProposeTask(c.Request.Context(), claims, spec)
	if err != nil {
		fail(c, err)
		return
	}
	if duplicate {
		c.JSON(http.StatusOK, gin.H{proposalIDKey: p.ID, statusKey: p.Status, "duplicate_proposal": true})
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	c.JSON(status, gin.H{proposalIDKey: p.ID, statusKey: p.Status, titleKey: p.Spec.Title, titleTruncatedKey: truncated})
}

// runtimeProposals lists the calling coordinator's own proposals.
func (h *Handler) runtimeProposals(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	status := ""
	switch c.Query("status") {
	case "", "all":
	case models.ProposalPending:
		status = models.ProposalPending
	default:
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "status must be pending or all"})
		return
	}
	rows, err := h.Service.ListProposals(c.Request.Context(), claims.WorkspaceID, claims.AgentProfileID, status, proposalListLimit)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{entriesKey: rows, nextCursorKey: ""})
}
