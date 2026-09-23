package runtime

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	executionModeExecute = "execute"
	taskIDKey            = "task_id"
	workspaceIDKey       = "workspace_id"
)

const (
	intentRevisionKey   = "intent_revision"
	entriesKey          = "entries"
	authorTypeAgent     = "agent"
	executionModeDesign = "design"
	errorResponseKey    = "error"
	statusFailed        = "failed"
	nextCursorKey       = "next_cursor"
	authorTypeUser      = "user"
	titleKey            = "title"
	statusKey           = "status"
	stateReview         = "REVIEW"
	stateCompleted      = "COMPLETED"
)

type Handler struct {
	Service   *Service
	Authorize func(context.Context, string) error
	// AuthorizeManage gates a person's writes to a coordinator conversation.
	// The coordinator acts with workspace-manage authority, so directing it
	// needs that authority too.
	AuthorizeManage func(context.Context, string) error
}

func RegisterRoutes(g *gin.RouterGroup, h *Handler) {
	g.GET("/runtime/capabilities", h.workspaceCapabilities)
	g.GET("/runtime/memory", h.runtimeMemory)
	g.POST("/runtime/memory", h.remember)
	g.DELETE("/runtime/memory/:id", h.forget)
	g.GET("/tasks/:id", h.conversation)
	g.GET("/tasks/:id/comments", h.comments)
	g.POST("/tasks/:id/comments", h.comment)
	g.POST("/tasks/:id/retry", h.retry)
	g.GET("/runtime/workspace", h.catalog)
	g.POST("/runtime/workspace/manage", h.manageWorkspace)
	g.GET("/runtime/tasks", h.workspaceTasks)
	g.GET("/runtime/metrics", h.metrics)
	g.GET("/runtime/proposals", h.runtimeProposals)
	g.GET("/runtime/tasks/:id/details", h.details)
	g.GET("/runtime/tasks/:id/content", h.taskContent)
	g.GET("/runtime/tasks/:id/permissions", h.taskPermissions)
	g.POST("/runtime/tasks", h.createTask)
	g.POST("/runtime/tasks/:id/manage", h.manageTask)
	g.POST("/runtime/tasks/:id/status", h.updateTask)
	g.POST("/runtime/tasks/:id/source-issue", h.updateSourceIssue)
	g.POST("/runtime/comments", h.runtimeComment)
}

func fail(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: err.Error()})
}
