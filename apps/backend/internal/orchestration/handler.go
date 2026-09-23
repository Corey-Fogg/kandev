// Package orchestration exposes workspace coordinators on core task execution.
package orchestration

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestration/personas"
	"github.com/kandev/kandev/internal/orchestration/repository/sqlite"
)

const (
	errorResponseKey        = "error"
	errOrchestratorNotFound = "orchestrator not found"
)

type Handler struct {
	Registry  *sqlite.Repository
	Repo      *sqlite.Repository
	Agents    *personas.Service
	Authorize func(context.Context, string) error
	// AuthorizeManage gates changes to a workspace's coordinators. A
	// coordinator acts with workspace-manage authority, so configuring one
	// needs that authority too.
	AuthorizeManage  func(context.Context, string) error
	RoleWrite        gin.HandlerFunc
	ValidateExecutor func(context.Context, string) error
	// Runtime serves metrics and task proposals; nil answers those routes 503.
	Runtime RuntimeFacade
}

// RuntimeFacade is the coordinator runtime as the human routes use it.
type RuntimeFacade interface {
	CoordinatorMetrics(ctx context.Context, workspaceID, agentID string, days int) (models.CoordinatorMetrics, error)
	ListProposals(ctx context.Context, workspaceID, agentID, status string, limit int) ([]models.TaskProposal, error)
	GetProposal(ctx context.Context, workspaceID, agentID, id string) (*models.TaskProposal, error)
	DecideProposal(ctx context.Context, d models.ProposalDecision) (*models.TaskProposal, string, bool, error)
}

func RegisterRoutes(group *gin.RouterGroup, h *Handler) {
	group.Use(func(c *gin.Context) {
		if _, present := c.Get("agent_caller"); present {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{errorResponseKey: "orchestrator configuration requires a user"})
			return
		}
		if ws := c.Param("wsId"); ws != "" && h.Authorize != nil {
			if err := h.Authorize(c.Request.Context(), ws); err != nil {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{errorResponseKey: "workspace not found"})
				return
			}
		}
		if !h.authorizeChange(c) {
			return
		}
		c.Next()
	})
	group.GET("/roles", h.listRoles)
	group.POST("/roles", h.roleWrite, h.saveRole)
	group.PUT("/roles/:roleId", h.roleWrite, h.saveRole)
	group.DELETE("/roles/:roleId", h.roleWrite, h.deleteRole)
	group.GET("/workspaces/:wsId/profiles", h.profiles)
	group.GET("/workspaces/:wsId/orchestrators", h.list)
	group.POST("/workspaces/:wsId/orchestrators", h.create)
	group.GET("/workspaces/:wsId/orchestrators/:id", h.get)
	group.GET("/workspaces/:wsId/orchestrators/:id/tasks", h.tasks)
	group.POST("/workspaces/:wsId/import/:id", h.importAgent)
	group.PUT("/workspaces/:wsId/orchestrators/:id", h.update)
	group.PATCH("/workspaces/:wsId/orchestrators/:id", h.patch)
	group.GET("/workspaces/:wsId/orchestrators/:id/metrics", h.metrics)
	group.GET("/workspaces/:wsId/orchestrators/:id/proposals", h.listProposals)
	group.GET("/workspaces/:wsId/orchestrators/:id/proposals/:proposalId", h.getProposal)
	group.POST("/workspaces/:wsId/orchestrators/:id/proposals/:proposalId/approve", h.approveProposal)
	group.POST("/workspaces/:wsId/orchestrators/:id/proposals/:proposalId/dismiss", h.dismissProposal)
	group.DELETE("/workspaces/:wsId/orchestrators/:id", h.remove)
	group.POST("/workspaces/:wsId/orchestrators/:id/conversation", h.conversation)
	group.POST("/workspaces/:wsId/orchestrators/:id/status", h.status)
}

// authorizeChange requires workspace-manage access for every change to a
// workspace's coordinators. Opening a conversation only ensures it exists,
// so readers can still see the chat.
func (h *Handler) authorizeChange(c *gin.Context) bool {
	ws := c.Param("wsId")
	if ws == "" || h.AuthorizeManage == nil || c.Request.Method == http.MethodGet || strings.HasSuffix(c.FullPath(), "/conversation") {
		return true
	}
	if err := h.AuthorizeManage(c.Request.Context(), ws); err != nil {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{errorResponseKey: "workspace manage access required"})
		return false
	}
	return true
}

func fail(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: err.Error()})
}
func (h *Handler) listRoles(c *gin.Context) {
	rows, err := h.Registry.ListOrchestratorRoles(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"roles": rows})
}
func (h *Handler) saveRole(c *gin.Context) {
	var row models.OrchestratorRole
	if err := c.ShouldBindJSON(&row); err != nil {
		fail(c, err)
		return
	}
	row.ID = c.Param("roleId")
	if row.ID == "" {
		row.ID = uuid.NewString()
	}
	if err := h.Registry.SaveOrchestratorRole(c.Request.Context(), &row); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}
func (h *Handler) deleteRole(c *gin.Context) {
	if err := h.Registry.DeleteOrchestratorRole(c.Request.Context(), c.Param("roleId")); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
func (h *Handler) scoped(c *gin.Context) *models.AgentInstance {
	id := c.Param("id")
	role, err := h.Registry.OrchestratorRoleID(c.Request.Context(), id)
	if err != nil || role == "" {
		c.JSON(http.StatusNotFound, gin.H{errorResponseKey: errOrchestratorNotFound})
		return nil
	}
	a, err := h.Agents.GetAgentInstance(c.Request.Context(), id)
	if err != nil || a.WorkspaceID != c.Param("wsId") {
		c.JSON(http.StatusNotFound, gin.H{errorResponseKey: errOrchestratorNotFound})
		return nil
	}
	return a
}
func (h *Handler) list(c *gin.Context) {
	ids, err := h.Registry.ListOrchestratorIDs(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		fail(c, err)
		return
	}
	rows := []any{}
	for _, id := range ids {
		a, e := h.Agents.GetAgentInstance(c.Request.Context(), id)
		if e != nil {
			fail(c, e)
			return
		}
		row, e := h.describe(c.Request.Context(), a)
		if e != nil {
			fail(c, e)
			return
		}
		rows = append(rows, row)
	}
	c.JSON(http.StatusOK, gin.H{"orchestrators": rows})
}
func (h *Handler) get(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	row, err := h.describe(c.Request.Context(), a)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}
func (h *Handler) remove(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	if err := h.Agents.DeleteAgentInstance(c.Request.Context(), a.ID); err != nil {
		fail(c, err)
		return
	}
	if err := h.Registry.UnregisterOrchestrator(c.Request.Context(), a.ID); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
func (h *Handler) conversation(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	row, err := h.Repo.EnsureAgentConversation(c.Request.Context(), a)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task_id": row.TaskID})
}
func (h *Handler) status(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	var req struct {
		Status models.AgentStatus `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	if req.Status != models.AgentStatusPaused && req.Status != models.AgentStatusIdle {
		c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: "select paused or idle"})
		return
	}
	if _, err := h.Agents.UpdateAgentStatus(c.Request.Context(), a.ID, req.Status, ""); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) profiles(c *gin.Context) {
	rows, err := h.Registry.ExecutionProfileDirectory(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"profiles": rows})
}

func (h *Handler) roleWrite(c *gin.Context) {
	if h.RoleWrite != nil {
		h.RoleWrite(c)
	} else {
		c.Next()
	}
}

func (h *Handler) tasks(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	tasks, err := h.Registry.OrchestratedTasks(c.Request.Context(), a.WorkspaceID, a.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}
