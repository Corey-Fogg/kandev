package runtime

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) catalog(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	result, err := h.Service.Manager.WorkspaceCatalog(c.Request.Context(), claims.WorkspaceID, c.Query("detail") == "full")
	if err != nil {
		fail(c, err)
		return
	}
	profiles, err := h.Service.Repo.ExecutionProfileDirectory(c.Request.Context(), claims.WorkspaceID)
	if err != nil {
		fail(c, err)
		return
	}
	if data, ok := result.(map[string]any); ok {
		data["execution_profiles"] = profiles
	}
	c.JSON(200, result)
}

func (h *Handler) details(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	result, err := h.Service.Manager.WorkspaceTaskDetails(c.Request.Context(), claims.WorkspaceID, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	if c.Query("include_result") == "false" {
		if data, ok := result.(map[string]any); ok {
			delete(data, "messages")
			delete(data, "session_results")
		}
	}
	c.JSON(200, result)
}

func (h *Handler) createTask(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	var req struct {
		ProjectID      string `json:"project_id"`
		Title          string `json:"title"`
		Description    string `json:"description"`
		WorkflowID     string `json:"workflow_id"`
		WorkflowStepID string `json:"workflow_step_id"`
		ExecutionMode  string `json:"execution_mode"`
		RepositoryID   string `json:"repository_id"`
		AssigneeID     string `json:"assignee"`
		ExternalID     string `json:"external_id"`
		ParentID       string `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	if req.ProjectID != "" {
		fail(c, fmt.Errorf("use a workspace workflow, not an Office project"))
		return
	}
	title, truncated := fitTaskTitle(req.Title)
	if truncated {
		req.Description = "Full title: " + strings.TrimSpace(req.Title) + "\n\n" + req.Description
	}
	if req.ExecutionMode != "" && req.ExecutionMode != executionModeExecute && req.ExecutionMode != executionModeDesign {
		c.AbortWithStatusJSON(422, gin.H{errorResponseKey: "execution_mode must be design or execute"})
		return
	}
	id, err := h.Service.Manager.CreateWorkspaceTask(c.Request.Context(), models.WorkspaceTaskSpec{WorkspaceID: claims.WorkspaceID, ChiefID: claims.AgentProfileID, WorkflowID: req.WorkflowID, WorkflowStepID: req.WorkflowStepID, ExecutionMode: req.ExecutionMode, RepositoryID: req.RepositoryID, AssigneeID: req.AssigneeID, Title: title, Description: req.Description, ExternalID: req.ExternalID, ParentID: req.ParentID})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, titleKey: title, titleTruncatedKey: truncated})
}

func (h *Handler) manageTask(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	var req models.WorkspaceTaskCommand
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	truncated := false
	if req.Title != nil {
		var title string
		title, truncated = fitTaskTitle(*req.Title)
		req.Title = &title
	}
	req.WorkspaceID = claims.WorkspaceID
	req.ChiefID = claims.AgentProfileID
	req.TaskID = c.Param("id")
	if err := h.Service.Manager.ManageWorkspaceTask(c.Request.Context(), req); err != nil {
		fail(c, err)
		return
	}
	if truncated {
		c.JSON(http.StatusOK, gin.H{"ok": true, titleKey: *req.Title, titleTruncatedKey: true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) updateTask(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	if err := h.Service.UpdateStatus(c.Request.Context(), claims.WorkspaceID, c.Param("id"), req.Status); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
