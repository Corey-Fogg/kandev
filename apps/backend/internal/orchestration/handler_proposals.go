package orchestration

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
)

const (
	proposalKey         = "proposal"
	proposalIDParam     = "proposalId"
	proposalNotFound    = "proposal not found"
	runtimeUnavailable  = "orchestration runtime is unavailable"
	maxProposalPageSize = 50
)

// runtimeScoped resolves the orchestrator of a runtime-backed route.
func (h *Handler) runtimeScoped(c *gin.Context) *models.AgentInstance {
	if h.Runtime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{errorResponseKey: runtimeUnavailable})
		return nil
	}
	return h.scoped(c)
}

func (h *Handler) listProposals(c *gin.Context) {
	a := h.runtimeScoped(c)
	if a == nil {
		return
	}
	status := ""
	switch c.Query("status") {
	case "", "all":
	case models.ProposalPending:
		status = models.ProposalPending
	default:
		c.JSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "status must be pending or all"})
		return
	}
	limit := maxProposalPageSize
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxProposalPageSize {
			c.JSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "limit must be 1 to 50"})
			return
		}
		limit = parsed
	}
	rows, err := h.Runtime.ListProposals(c.Request.Context(), c.Param("wsId"), a.ID, status, limit)
	if err != nil {
		proposalFailure(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"proposals": rows})
}

func (h *Handler) getProposal(c *gin.Context) {
	a := h.runtimeScoped(c)
	if a == nil {
		return
	}
	p, err := h.Runtime.GetProposal(c.Request.Context(), c.Param("wsId"), a.ID, c.Param(proposalIDParam))
	if err != nil {
		proposalFailure(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) approveProposal(c *gin.Context) {
	a := h.runtimeScoped(c)
	if a == nil {
		return
	}
	var req struct {
		Edits *models.ProposalEdits `json:"edits"`
	}
	if !bindOptionalJSON(c, &req) {
		return
	}
	p, taskID, duplicate, err := h.Runtime.DecideProposal(c.Request.Context(), h.decision(c, a.ID, models.ProposalActionApprove, req.Edits, ""))
	if err != nil {
		proposalFailure(c, p, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{proposalKey: p, "task_id": taskID, "duplicate": duplicate})
}

func (h *Handler) dismissProposal(c *gin.Context) {
	a := h.runtimeScoped(c)
	if a == nil {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !bindOptionalJSON(c, &req) {
		return
	}
	p, _, _, err := h.Runtime.DecideProposal(c.Request.Context(), h.decision(c, a.ID, models.ProposalActionDismiss, nil, req.Reason))
	if err != nil {
		proposalFailure(c, p, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{proposalKey: p})
}

// decision builds a decision whose scope comes only from the route and the
// signed-in user, never from the body.
func (h *Handler) decision(c *gin.Context, agentID, action string, edits *models.ProposalEdits, reason string) models.ProposalDecision {
	identity, _ := authn.FromGin(c)
	return models.ProposalDecision{WorkspaceID: c.Param("wsId"), OrchestratorID: agentID, ProposalID: c.Param(proposalIDParam),
		UserID: identity.UserID, Action: action, Edits: edits, Reason: reason}
}

// bindOptionalJSON reads a JSON body that may be empty.
func bindOptionalJSON(c *gin.Context, dest any) bool {
	if c.Request.ContentLength == 0 {
		return true
	}
	if err := c.ShouldBindJSON(dest); err != nil {
		fail(c, err)
		return false
	}
	return true
}

// proposalFailure maps a proposal error to its response.
func proposalFailure(c *gin.Context, p *models.TaskProposal, err error) {
	var input *models.ProposalInputError
	switch {
	case errors.Is(err, models.ErrProposalNotFound):
		c.JSON(http.StatusNotFound, gin.H{errorResponseKey: proposalNotFound})
	case errors.Is(err, models.ErrProposalDecided):
		c.JSON(http.StatusConflict, gin.H{errorResponseKey: models.ErrProposalDecided.Error(), proposalKey: p})
	case errors.As(err, &input):
		c.JSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: input.Error()})
	default:
		fail(c, err)
	}
}
