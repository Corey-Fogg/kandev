package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/orchestration"
	orchestrationstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
)

func registerOrchestration(p routeParams) {
	if !p.features.Orchestration || p.services.Orchestration == nil {
		return
	}
	group := p.router.Group("/api/v1/orchestration", runtimeauth.Middleware(p.services.Orchestration.Auth, p.services.Orchestration.Personas))
	// Runtime routes register before orchestration.RegisterRoutes adds its
	// user-only configuration middleware to the shared group.
	// A coordinator acts with workspace-manage authority, so configuring or
	// messaging one requires it; reading needs workspace access only.
	manage := func(ctx context.Context, ws string) error {
		return p.taskSvc.AuthorizeWorkspaceScope(ctx, ws, authz.ScopeWorkspaceManage)
	}
	orchestrationruntime.RegisterRoutes(group, &orchestrationruntime.Handler{Service: p.services.Orchestration, Authorize: p.taskSvc.AuthorizeWorkspaceAccess, AuthorizeManage: manage})
	orchestration.RegisterRoutes(group, &orchestration.Handler{Registry: p.orchestrationRepo, Repo: p.orchestrationRepo, Agents: p.services.Orchestration.Personas, Authorize: p.taskSvc.AuthorizeWorkspaceAccess, AuthorizeManage: manage, RoleWrite: authn.RequireAdmin(), ValidateExecutor: func(ctx context.Context, raw string) error {
		var preference struct {
			ID string `json:"executor_profile_id"`
		}
		if err := json.Unmarshal([]byte(raw), &preference); err != nil || preference.ID == "" {
			return fmt.Errorf("select an executor profile")
		}
		_, err := p.taskSvc.GetExecutorProfile(ctx, preference.ID)
		return err
	}})
}

// Office APIs never own registered Orchestration personas or conversations.
func orchestrationCompatibilityGate(p routeParams) gin.HandlerFunc {
	return func(c *gin.Context) {
		caller := agents.CallerFromContext(c)
		if caller != nil {
			legacy, err := legacyOfficePersona(c.Request.Context(), p.orchestrationRepo, caller.ID)
			allowed := p.features.Office && legacy
			if err != nil || !allowed {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{errKey: "feature disabled"})
				return
			}
			c.Next()
			return
		}
		allowed, err := orchestrationBrowserRouteAllowed(c.Request.Context(), p, c.Request.URL.Path)
		if err != nil || !allowed {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{errKey: "feature disabled"})
			return
		}
		c.Next()
	}
}

func orchestrationBrowserRouteAllowed(ctx context.Context, p routeParams, path string) (bool, error) {
	if !p.features.Office {
		return false, nil
	}
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/office/"), "/")
	if len(parts) < 2 {
		return true, nil
	}
	id := ""
	switch parts[0] {
	case "agents":
		id = parts[1]
	case workspaceTasksKey:
		fields, err := p.officeRepo.GetTaskExecutionFields(ctx, parts[1])
		if err != nil {
			// A task this gate cannot read is not a coordinator's: the Office
			// handler answers for it with its own not-found or error.
			return true, nil
		}
		id = fields.AssigneeAgentProfileID
	default:
		return true, nil
	}
	return legacyOfficePersona(ctx, p.orchestrationRepo, id)
}

func legacyOfficePersona(ctx context.Context, repo *orchestrationstore.Repository, id string) (bool, error) {
	if id == "" {
		return true, nil
	}
	role, err := repo.OrchestratorRoleID(ctx, id)
	return role == "", err
}
