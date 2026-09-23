package runtime

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

const (
	// coordinatorMemoryLayer holds every entry the coordinator records.
	coordinatorMemoryLayer = "workspace"
	// maxCoordinatorMemories bounds the entries one coordinator keeps.
	maxCoordinatorMemories = 50
	// maxMemoryContentBytes keeps every entry whole inside the prompt budget.
	maxMemoryContentBytes = 2000
	maxMemoryKeyBytes     = 200
)

// runtimeMemory lists the calling coordinator's workspace memory.
func (h *Handler) runtimeMemory(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	rows, err := h.Service.Repo.ListAgentMemory(c.Request.Context(), claims.AgentProfileID)
	if err != nil {
		fail(c, err)
		return
	}
	filtered := make([]*models.AgentMemory, 0, len(rows))
	for _, row := range rows {
		if id := c.Query("memory_id"); id != "" && row.ID != id {
			continue
		}
		if key := c.Query("key"); key != "" && row.Key != key {
			continue
		}
		filtered = append(filtered, row)
	}
	c.JSON(http.StatusOK, gin.H{entriesKey: filtered, "count": len(filtered)})
}

// remember stores or replaces one entry of the calling coordinator's memory.
func (h *Handler) remember(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	var req struct {
		Key     string `json:"key"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	req.Key, req.Content = strings.TrimSpace(req.Key), strings.TrimSpace(req.Content)
	if req.Key == "" || len(req.Key) > maxMemoryKeyBytes || req.Content == "" || len(req.Content) > maxMemoryContentBytes || !utf8.ValidString(req.Content) {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: fmt.Sprintf("key (1-%d bytes) and content (1-%d bytes) are required", maxMemoryKeyBytes, maxMemoryContentBytes)})
		return
	}
	ctx := c.Request.Context()
	rows, err := h.Service.Repo.ListAgentMemory(ctx, claims.AgentProfileID)
	if err != nil {
		fail(c, err)
		return
	}
	if len(rows) >= maxCoordinatorMemories && !memoryKeyExists(rows, req.Key) {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: fmt.Sprintf("memory holds %d entries; forget one before adding another", maxCoordinatorMemories)})
		return
	}
	row := &models.AgentMemory{AgentProfileID: claims.AgentProfileID, Layer: coordinatorMemoryLayer, Key: req.Key, Content: req.Content, Metadata: "{}"}
	if err := h.Service.Repo.UpsertAgentMemory(ctx, row); err != nil {
		fail(c, err)
		return
	}
	saved, err := h.Service.Repo.GetAgentMemory(ctx, claims.AgentProfileID, coordinatorMemoryLayer, req.Key)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, saved)
}

// forget deletes one entry of the calling coordinator's memory.
func (h *Handler) forget(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	if err := h.Service.Repo.DeleteAgentMemoryOwned(c.Request.Context(), claims.AgentProfileID, c.Param("id")); err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{errorResponseKey: "memory entry not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"forgotten": true})
}

func memoryKeyExists(rows []*models.AgentMemory, key string) bool {
	for _, row := range rows {
		if row.Layer == coordinatorMemoryLayer && row.Key == key {
			return true
		}
	}
	return false
}
