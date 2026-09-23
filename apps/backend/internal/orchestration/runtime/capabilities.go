package runtime

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// workspaceCapabilities lists the broker tools a coordinator may call.
func (h *Handler) workspaceCapabilities(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	limit, ok := boundedPageLimit(c)
	if !ok {
		return
	}
	scope := cursorScope("workspace-controls-v1", claims.TaskID, claims.WorkspaceID)
	after, err := decodeScopedCursor(c.Query("after"), scope)
	if err != nil {
		fail(c, err)
		return
	}
	tools := models.WorkspaceBrokerTools()
	slices.SortFunc(tools, func(a, b models.WorkspaceBrokerTool) int { return strings.Compare(a.Name, b.Name) })
	page := models.CapabilityPage{Entries: []models.Capability{}}
	for _, tool := range tools {
		if tool.Name <= after {
			continue
		}
		if len(page.Entries) == limit {
			page.NextCursor = encodeScopedCursor(scope, page.Entries[len(page.Entries)-1].Name)
			break
		}
		effect := "read"
		if tool.Method != http.MethodGet {
			effect = "write"
		}
		page.Entries = append(page.Entries, models.Capability{Name: tool.Name, Description: tool.Description, Effect: effect})
	}
	c.JSON(http.StatusOK, page)
}
