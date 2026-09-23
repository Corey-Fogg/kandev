package runtime

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestration/models"
)

const (
	runtimeCommentLimit     = 10
	runtimeCommentMaxLimit  = 50
	runtimeCommentBodyRunes = 1500
)

func (h *Handler) conversation(c *gin.Context) {
	if _, ok := c.Get("agent_claims"); ok {
		h.runtimeTask(c)
		return
	}
	owner, _, ok := h.scopedConversation(c)
	if !ok {
		return
	}
	task, err := h.Service.Tasks.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"id": task.ID, titleKey: task.Title, "workspace_id": task.WorkspaceID, "orchestrator_id": owner})
}

// comments pages a conversation's messages, newest last. A coordinator reads
// bounded pages with clipped bodies and fetches one message in full by id.
func (h *Handler) comments(c *gin.Context) {
	if !h.canReadTask(c) {
		return
	}
	_, runtimeCaller := c.Get("agent_claims")
	if id := c.Query("comment_id"); runtimeCaller && id != "" {
		row, err := h.Service.Repo.GetCommentByID(c.Request.Context(), c.Param("id"), id)
		if err != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.JSON(http.StatusOK, gin.H{"comment": row})
		return
	}
	limit, maxLimit := 500, 500
	if runtimeCaller {
		limit, maxLimit = runtimeCommentLimit, runtimeCommentMaxLimit
	}
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxLimit {
			fail(c, fmt.Errorf("limit must be between 1 and %d", maxLimit))
			return
		}
		limit = n
	}
	rows, nextCursor, err := h.commentPage(c, limit)
	if err != nil {
		fail(c, err)
		return
	}
	if runtimeCaller {
		c.JSON(http.StatusOK, gin.H{"comments": clipComments(rows), nextCursorKey: nextCursor})
		return
	}
	c.JSON(http.StatusOK, gin.H{"comments": rows, nextCursorKey: nextCursor})
}

// commentPage reads up to limit messages before the requested cursor with
// their run status, oldest first.
func (h *Handler) commentPage(c *gin.Context, limit int) ([]*models.TaskComment, string, error) {
	rows, err := h.Service.Repo.CommentsBefore(c.Request.Context(), c.Param("id"), c.Query("before"), limit+1)
	if err != nil {
		return nil, "", err
	}
	nextCursor := ""
	if len(rows) > limit {
		rows = rows[:limit]
		nextCursor = rows[len(rows)-1].ID
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	statuses, err := h.Service.Runs.GetRunsByCommentIDs(c.Request.Context(), ids)
	if err != nil {
		return nil, "", err
	}
	for _, row := range rows {
		if run, ok := statuses[row.ID]; ok {
			row.RunID = run.RunID
			row.RunStatus = run.Status
			row.RunError = run.ErrorMessage
		}
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, nextCursor, nil
}

// clippedComment is a coordinator's view of a message: bodies beyond
// runtimeCommentBodyRunes are cut and flagged.
type clippedComment struct {
	*models.TaskComment
	Body      string `json:"body"`
	Truncated bool   `json:"truncated,omitempty"`
}

func clipComments(rows []*models.TaskComment) []clippedComment {
	clipped := make([]clippedComment, 0, len(rows))
	for _, row := range rows {
		body, truncated := clipRunes(row.Body, runtimeCommentBodyRunes)
		clipped = append(clipped, clippedComment{TaskComment: row, Body: body, Truncated: truncated})
	}
	return clipped
}

func (h *Handler) comment(c *gin.Context) {
	if _, ok := c.Get("agent_claims"); ok {
		h.runtimeComment(c)
		return
	}
	owner, _, ok := h.scopedConversation(c)
	if !ok {
		return
	}
	persona, err := h.Service.Personas.GetAgentInstance(c.Request.Context(), owner)
	if err != nil {
		fail(c, err)
		return
	}
	if paused(persona) {
		fail(c, fmt.Errorf("coordinator is paused"))
		return
	}
	h.acceptComment(c, owner)
}

func (h *Handler) runtimeComment(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	var req struct {
		TaskID string `json:"task_id"`
		Body   string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	if id := c.Param("id"); id != "" {
		req.TaskID = id
	}
	if req.TaskID == "" {
		req.TaskID = claims.TaskID
	}
	task, err := h.Service.Tasks.GetTask(c.Request.Context(), req.TaskID)
	if err != nil || task.WorkspaceID != claims.WorkspaceID {
		c.AbortWithStatus(403)
		return
	}
	if len(req.Body) == 0 || len(req.Body) > 32000 {
		fail(c, fmt.Errorf("invalid comment length"))
		return
	}
	row := &models.TaskComment{TaskID: req.TaskID, AuthorID: claims.AgentProfileID, AuthorType: authorTypeAgent, Body: req.Body, Source: authorTypeAgent}
	if err := h.Service.Repo.PutComment(c.Request.Context(), row); err != nil {
		fail(c, err)
		return
	}
	c.JSON(201, row)
}

func (h *Handler) runtimeTask(c *gin.Context) {
	if !h.authorizeRuntimeTask(c) {
		return
	}
	task, err := h.Service.Tasks.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, task)
}
