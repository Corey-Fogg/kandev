package runtime

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

func (h *Handler) workspaceTasks(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	limit, ok := boundedPageLimit(c)
	if !ok {
		return
	}
	scope := cursorScope("workspace-tasks-v1", claims.TaskID, claims.WorkspaceID, "", strconv.Itoa(limit))
	after, err := decodeScopedCursor(c.Query("after"), scope)
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	page := 1
	if after != "" {
		page, err = strconv.Atoi(after)
		if err != nil || page < 1 || page > 100000 {
			c.AbortWithStatus(400)
			return
		}
	}
	rows, more, err := h.Service.Manager.WorkspaceTaskSummaries(c.Request.Context(), claims.WorkspaceID, page, limit)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	next := ""
	if more {
		next = encodeScopedCursor(scope, strconv.Itoa(page+1))
	}
	c.JSON(200, gin.H{workspaceIDKey: claims.WorkspaceID, entriesKey: rows, nextCursorKey: next})
}
