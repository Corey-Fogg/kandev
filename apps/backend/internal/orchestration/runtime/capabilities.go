package runtime

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// workspaceCapabilities lists the broker controls a coordinator may call.
func (h *Handler) workspaceCapabilities(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	limit, ok := boundedPageLimit(c)
	if !ok {
		return
	}
	kind := c.Query("kind")
	if kind != "" && kind != "native" {
		c.JSON(400, gin.H{errorResponseKey: "workspace capabilities supports kind=native; use workspace for resource IDs"})
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
	page := models.CapabilityPage{Entries: []models.Capability{}, Generation: "workspace-controls-v1"}
	for _, tool := range tools {
		if tool.Name <= after {
			continue
		}
		if len(page.Entries) == limit {
			page.NextCursor = encodeScopedCursor(scope, page.Entries[len(page.Entries)-1].Name)
			break
		}
		const workspaceReadEffect = "read"
		effect := workspaceReadEffect
		if tool.Method != http.MethodGet {
			effect = "write"
		}
		page.Entries = append(page.Entries, models.Capability{ID: "native/" + tool.Name, Kind: "native", Name: tool.Name,
			WorkspaceID: claims.WorkspaceID, Effect: effect, Surfaces: []string{"conversation"}, Health: "ready",
			Configured: true, Attached: true, InspectAllowed: true, Reason: tool.Description, Revision: page.Generation,
			InputSchema: json.RawMessage(`{"type":"object"}`), SchemaPartial: true})
	}
	c.JSON(200, page)
}
