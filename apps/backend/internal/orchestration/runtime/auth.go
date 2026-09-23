package runtime

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/runtimeauth"
)

// callerClaimsKey caches the claims a request already authorized, so nested
// helpers reuse one run, role, intent and workspace check per request.
const callerClaimsKey = "orchestration_caller_claims"

func (h *Handler) caller(c *gin.Context) (*runtimeauth.AgentClaims, bool) {
	if cached, ok := c.Get(callerClaimsKey); ok {
		return cached.(*runtimeauth.AgentClaims), true
	}
	claims, ok := h.authorizeCaller(c)
	if ok {
		c.Set(callerClaimsKey, claims)
	}
	return claims, ok
}

func (h *Handler) authorizeCaller(c *gin.Context) (*runtimeauth.AgentClaims, bool) {
	raw, ok := c.Get("agent_claims")
	claims, valid := raw.(*runtimeauth.AgentClaims)
	if !ok || !valid || claims.Capabilities != workspaceCoordinatorAudience {
		c.AbortWithStatusJSON(403, gin.H{errorResponseKey: "coordinator token required"})
		return nil, false
	}
	run, err := h.Service.Runs.GetRunByID(c.Request.Context(), claims.RunID)
	if err != nil || run == nil || run.AgentProfileID != claims.AgentProfileID || run.Status != statusClaimed || run.SessionID != claims.SessionID {
		c.AbortWithStatusJSON(403, gin.H{errorResponseKey: "run is no longer active"})
		return nil, false
	}
	role, err := h.Service.Repo.OrchestratorRoleID(c.Request.Context(), claims.AgentProfileID)
	if err != nil || role == "" {
		c.AbortWithStatus(403)
		return nil, false
	}
	if !h.currentRunAuthority(c, claims, run.Payload) {
		return nil, false
	}
	// A coordinator credential never reaches beyond its signed home workspace.
	if workspace := c.Query(workspaceIDKey); workspace != "" && workspace != claims.WorkspaceID {
		c.AbortWithStatus(http.StatusForbidden)
		return nil, false
	}
	return claims, true
}

func (h *Handler) currentRunAuthority(c *gin.Context, claims *runtimeauth.AgentClaims, payload string) bool {
	if c.Request.Method == http.MethodGet {
		return true
	}
	if c.GetHeader("X-Kandev-Run-Id") != claims.RunID {
		c.AbortWithStatus(http.StatusForbidden)
		return false
	}
	return h.currentIntent(c, claims.TaskID, payload)
}

func (h *Handler) scopedConversation(c *gin.Context) (string, string, bool) {
	id := c.Param("id")
	owner, ws, err := h.Service.Repo.ConversationOwner(c.Request.Context(), id)
	if err != nil {
		c.AbortWithStatus(404)
		return "", "", false
	}
	if _, ok := c.Get("agent_claims"); ok {
		claims, valid := h.caller(c)
		if !valid {
			return "", "", false
		}
		if claims.AgentProfileID != owner || claims.WorkspaceID != ws {
			c.AbortWithStatus(403)
			return "", "", false
		}
	} else if h.Authorize != nil {
		if err := h.Authorize(c.Request.Context(), ws); err != nil {
			c.AbortWithStatus(404)
			return "", "", false
		}
		if c.Request.Method != http.MethodGet && h.AuthorizeManage != nil {
			if err := h.AuthorizeManage(c.Request.Context(), ws); err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{errorResponseKey: "workspace manage access required"})
				return "", "", false
			}
		}
	}
	return owner, ws, true
}

func (h *Handler) canReadTask(c *gin.Context) bool {
	if _, ok := c.Get("agent_claims"); ok {
		return h.authorizeRuntimeTask(c)
	}
	_, _, ok := h.scopedConversation(c)
	return ok
}

func (h *Handler) authorizeRuntimeTask(c *gin.Context) bool {
	claims, ok := h.caller(c)
	if !ok {
		return false
	}
	task, err := h.Service.Tasks.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil || task.WorkspaceID != claims.WorkspaceID {
		c.AbortWithStatus(404)
		return false
	}
	return true
}
