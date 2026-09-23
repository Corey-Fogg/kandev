package runtime

import (
	"errors"
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

// createTaskRequest is the create_task body.
type createTaskRequest struct {
	ProjectID          string              `json:"project_id"`
	Title              string              `json:"title"`
	Description        string              `json:"description"`
	WorkflowID         string              `json:"workflow_id"`
	WorkflowStepID     string              `json:"workflow_step_id"`
	ExecutionMode      string              `json:"execution_mode"`
	RepositoryID       string              `json:"repository_id"`
	AssigneeID         string              `json:"assignee"`
	ExternalID         string              `json:"external_id"`
	ParentID           string              `json:"parent_id"`
	Source             *models.SourceIssue `json:"source"`
	AcceptanceCriteria []string            `json:"acceptance_criteria"`
}

func (h *Handler) createTask(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	spec, truncated, ok := h.createTaskSpec(c, claims.WorkspaceID)
	if !ok {
		return
	}
	goal, err := models.NewTaskGoal(spec.AcceptanceCriteria, h.Service.now())
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: err.Error()})
		return
	}
	assignment, err := h.Service.assignment(c.Request.Context(), claims.AgentProfileID)
	if err != nil {
		fail(c, err)
		return
	}
	if assignment.AskBeforeCreate {
		h.proposeTask(c, claims, spec, truncated)
		return
	}
	id, err := h.Service.Manager.CreateWorkspaceTask(c.Request.Context(), workspaceTaskSpec(claims.WorkspaceID, claims.AgentProfileID, spec, goal))
	var duplicate *models.DuplicateTaskError
	if errors.As(err, &duplicate) {
		c.JSON(http.StatusOK, gin.H{"id": duplicate.TaskID, duplicateKey: true, archivedKey: duplicate.Archived})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, titleKey: spec.Title, titleTruncatedKey: truncated})
}

// createTaskSpec reads and validates a create_task request. It answers the
// request itself when it is invalid or its source issue already has a task.
func (h *Handler) createTaskSpec(c *gin.Context, workspaceID string) (models.ProposalSpec, bool, bool) {
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return models.ProposalSpec{}, false, false
	}
	if req.ProjectID != "" {
		fail(c, fmt.Errorf("use a workspace workflow, not an Office project"))
		return models.ProposalSpec{}, false, false
	}
	title, truncated := fitTaskTitle(req.Title)
	if truncated {
		req.Description = "Full title: " + strings.TrimSpace(req.Title) + "\n\n" + req.Description
	}
	if req.ExecutionMode != "" && req.ExecutionMode != executionModeExecute && req.ExecutionMode != executionModeDesign {
		c.AbortWithStatusJSON(422, gin.H{errorResponseKey: "execution_mode must be design or execute"})
		return models.ProposalSpec{}, false, false
	}
	if req.Source != nil {
		source, ok := h.sourceForCreate(c, workspaceID, *req.Source, req.ExternalID)
		if !ok {
			return models.ProposalSpec{}, false, false
		}
		req.Source = &source
	}
	return models.ProposalSpec{Title: title, Description: req.Description, WorkflowID: req.WorkflowID, WorkflowStepID: req.WorkflowStepID,
		RepositoryID: req.RepositoryID, ParentID: req.ParentID, AssigneeID: req.AssigneeID, ExecutionMode: req.ExecutionMode,
		ExternalID: req.ExternalID, Source: req.Source, AcceptanceCriteria: req.AcceptanceCriteria}, truncated, true
}

// sourceForCreate validates a create_task source issue and answers with the
// existing task when the workspace already has one for that issue.
func (h *Handler) sourceForCreate(c *gin.Context, workspaceID string, raw models.SourceIssue, externalID string) (models.SourceIssue, bool) {
	source, err := models.NormalizeSourceIssue(raw)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: err.Error()})
		return source, false
	}
	if externalID != "" && strings.TrimSpace(externalID) != source.ExternalID() {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: "external_id must be omitted or equal " + source.ExternalID() + " when source is set"})
		return source, false
	}
	existing, archived, err := h.Service.Repo.TaskForSourceIssue(c.Request.Context(), workspaceID, source.MetadataKey(), source.Key, source.ExternalID())
	if err != nil {
		fail(c, err)
		return source, false
	}
	if existing != "" {
		c.JSON(http.StatusOK, gin.H{"id": existing, duplicateKey: true, archivedKey: archived})
		return source, false
	}
	return source, true
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
	if h.manageCriteria(c, claims, req) {
		return
	}
	req.WorkspaceID = claims.WorkspaceID
	req.ChiefID = claims.AgentProfileID
	req.TaskID = c.Param("id")
	if err := h.Service.Manager.ManageWorkspaceTask(c.Request.Context(), req); err != nil {
		if errors.Is(err, models.ErrCriteriaUnmet) {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{errorResponseKey: err.Error()})
			return
		}
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
	if !h.completionAllowed(c, claims, c.Param("id"), req.Status) {
		return
	}
	if err := h.Service.UpdateStatus(c.Request.Context(), claims.WorkspaceID, c.Param("id"), req.Status); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
