package orchestration

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestration/models"
)

const orchestratorIDKey = "orchestrator_id"

// rejectExisting answers 409 when the workspace already has an orchestrator
// other than selfID. A workspace has one orchestrator; legacy extras keep
// working but no new one is added.
func (h *Handler) rejectExisting(c *gin.Context, selfID string) bool {
	ids, err := h.Registry.ListOrchestratorIDs(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		fail(c, err)
		return true
	}
	for _, id := range ids {
		if id != selfID {
			c.JSON(http.StatusConflict, gin.H{errorResponseKey: models.ErrOrchestratorExists.Error(), orchestratorIDKey: id})
			return true
		}
	}
	return false
}

// failConfiguration answers a failed registration.
func (h *Handler) failConfiguration(c *gin.Context, err error) {
	if !errors.Is(err, models.ErrOrchestratorExists) || !h.rejectExisting(c, "") {
		fail(c, err)
	}
}

// respondDescribed answers with the orchestrator as it is stored now.
func (h *Handler) respondDescribed(c *gin.Context, status int, id string) {
	a, err := h.Agents.GetAgentInstance(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	row, err := h.describe(c.Request.Context(), a)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(status, row)
}

// patch renames an orchestrator or changes its settings. Unlike a full
// update it is allowed while the orchestrator is working.
func (h *Handler) patch(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	var req models.OrchestratorPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	if req.Empty() {
		c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: "change display_name or a setting"})
		return
	}
	ctx := c.Request.Context()
	if req.DisplayName != nil {
		name, err := models.NormalizeDisplayName(*req.DisplayName)
		if err != nil {
			fail(c, err)
			return
		}
		if _, err := h.Registry.SetOrchestratorDisplayName(ctx, a.ID, name); err != nil {
			fail(c, err)
			return
		}
	}
	if req.AskBeforeCreate != nil || req.AutoCommentSource != nil || req.AutoMoveSourceDone != nil {
		stored, err := h.Registry.OrchestratorAssignment(ctx, a.ID)
		if err != nil || stored == nil {
			fail(c, errors.Join(err, errors.New("orchestrator is not registered")))
			return
		}
		if err := h.Registry.SaveOrchestratorSettings(ctx, a.ID, req.ApplySettings(stored.OrchestratorSettings)); err != nil {
			fail(c, err)
			return
		}
	}
	h.respondDescribed(c, http.StatusOK, a.ID)
}
