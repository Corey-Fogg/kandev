package runtime

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	orchestrationapi "github.com/kandev/kandev/internal/orchestration"
	"github.com/stretchr/testify/require"
)

func TestAssistantPrivacyRefusesInactiveTurns(t *testing.T) {
	s, _, task := newRuntime(t)
	human := assistantRouter(s)
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	switchPrivateAssistant(t, s, human, 1)
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	response := runtimeRequest(t, human, "POST", path, "", "", map[string]string{"body": "Old conversation"})
	require.Equal(t, 409, response.Code, response.Body.String())
	comments, err := s.Repo.ListComments(context.Background(), task, 10)
	require.NoError(t, err)
	require.Empty(t, comments)
}

func TestAssistantPrivacyAutomationRequiresDurableOwnerAuthority(t *testing.T) {
	s, _, task := newRuntime(t)
	human := assistantRouter(s)
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	for _, phase := range []string{"current", "previous"} {
		if phase == "previous" {
			switchPrivateAssistant(t, s, human, 1)
		}
		for _, ctx := range []context.Context{
			context.Background(),
			authn.WithIdentity(context.Background(), authn.Identity{UserID: "foreign"}),
		} {
			require.Error(t, s.Validate(ctx, "ws", "chief"), phase)
			_, err := s.Send(ctx, "ws", "chief", phase, "Untrusted automation")
			require.Error(t, err)
		}
	}
	comments, err := s.Repo.ListComments(context.Background(), task, 10)
	require.NoError(t, err)
	require.Empty(t, comments)
}

func TestAssistantPrivacyConfigurationUsesRetainedOwner(t *testing.T) {
	s, _, _ := newRuntime(t)
	human := assistantRouter(s)
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	switchPrivateAssistant(t, s, human, 1)
	foreign, owner := privateConfigurationRouter(s, "foreign"), privateConfigurationRouter(s, "owner")
	base := "/api/v1/orchestration/workspaces/ws/orchestrators"
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		r := runtimeRequest(t, foreign, method, base+"/chief", "", "", map[string]string{"profile_id": "personal", "role_id": "chief-of-staff"})
		require.Equal(t, 404, r.Code, r.Body.String())
	}
	for _, suffix := range []string{"conversation", "status"} {
		r := runtimeRequest(t, foreign, "POST", base+"/chief/"+suffix, "", "", map[string]string{"status": "paused"})
		require.Equal(t, 404, r.Code, r.Body.String())
	}
	listed := runtimeRequest(t, foreign, "GET", base, "", "", nil)
	require.Equal(t, 200, listed.Code)
	require.NotContains(t, listed.Body.String(), "chief")
	require.Equal(t, 200, runtimeRequest(t, owner, "GET", base+"/chief", "", "", nil).Code)
	require.NoError(t, s.Repo.UnregisterOrchestrator(context.Background(), "chief"))
	imported := runtimeRequest(t, foreign, "POST", "/api/v1/orchestration/workspaces/ws/import/chief", "", "", map[string]string{"profile_id": "personal", "role_id": "chief-of-staff"})
	require.Equal(t, 404, imported.Code, imported.Body.String())
}

func privateConfigurationRouter(s *Service, user string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: user, Role: authn.RoleMember})
	})
	orchestrationapi.RegisterRoutes(router.Group("/api/v1/orchestration"), &orchestrationapi.Handler{Registry: s.Repo, Repo: s.Repo, Agents: s.Personas})
	return router
}
