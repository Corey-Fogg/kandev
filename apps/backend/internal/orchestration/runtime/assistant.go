package runtime

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func assistantHuman(c *gin.Context) (authn.Identity, bool) {
	if _, agent := c.Get("agent_claims"); agent {
		c.AbortWithStatus(http.StatusForbidden)
		return authn.Identity{}, false
	}
	identity, ok := authn.FromGin(c)
	if !ok || identity.UserID == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return identity, false
	}
	return identity, true
}

func (h *Handler) assistantWorkspaceAllowed(c *gin.Context, workspaceID string) bool {
	return h.Authorize == nil || h.Authorize(c.Request.Context(), workspaceID) == nil
}

// humanAssistant resolves the caller's binding for the binding-keyed human
// surfaces. Coordinator conversations never consult it.
func (h *Handler) humanAssistant(c *gin.Context) (*models.AssistantBinding, bool) {
	identity, ok := assistantHuman(c)
	if !ok {
		return nil, false
	}
	row, err := h.Service.Repo.AssistantBinding(c.Request.Context(), identity.UserID)
	if err != nil || !h.assistantWorkspaceAllowed(c, row.WorkspaceID) {
		c.AbortWithStatus(http.StatusNotFound)
		return nil, false
	}
	return row, true
}
