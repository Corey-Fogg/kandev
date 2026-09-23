package runtime

import (
	"context"
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

type assistantTaskManager struct {
	creates  atomic.Int64
	failure  error
	started  chan struct{}
	release  chan struct{}
	lastSpec models.WorkspaceTaskSpec
}

func (m *assistantTaskManager) CreateWorkspaceTask(_ context.Context, spec models.WorkspaceTaskSpec) (string, error) {
	m.creates.Add(1)
	m.lastSpec = spec
	if m.started != nil {
		close(m.started)
		<-m.release
	}
	return "created-task", m.failure
}

func (m *assistantTaskManager) ManageWorkspaceTask(context.Context, models.WorkspaceTaskCommand) error {
	return nil
}
func (m *assistantTaskManager) WorkspaceTaskDetails(context.Context, string, string) (any, error) {
	return nil, nil
}
func (m *assistantTaskManager) WorkspaceCatalog(context.Context, string) (any, error) {
	return nil, nil
}

func assistantRuntimeCaller(t *testing.T, s *Service, task string) (*gin.Engine, string, string) {
	t.Helper()
	return assistantRuntimeCallerMode(t, s, task, "execute")
}

func assistantRuntimeCallerMode(t *testing.T, s *Service, task, mode string) (*gin.Engine, string, string) {
	t.Helper()
	ctx := context.Background()
	require.Equal(t, 200, runtimeRequest(t, assistantRouter(s), "PUT", "/api/v1/orchestration/assistant",
		"", "", map[string]any{"orchestrator_id": "chief", "execution_mode": mode}).Code)
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "operation-run", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, workspaceCoordinatorAudience, run.Payload, "session"))
	token, err := s.Auth.MintRuntimeJWT("chief", task, "ws", run.ID, "session", workspaceCoordinatorAudience)
	require.NoError(t, err)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/orchestration", runtimeauth.Middleware(s.Auth, s.Personas)), &Handler{Service: s})
	return router, token, run.ID
}

func TestCoordinatorCreateIgnoresRetainedBinding(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	result := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, map[string]any{"title": "Plain delegation"})
	require.Equal(t, 201, result.Code, result.Body.String())
	require.EqualValues(t, 1, manager.creates.Load())
	result = runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, map[string]any{"title": "Missing intent", "operation_id": "no-intent"})
	require.Equal(t, 422, result.Code, "a ledgered operation still requires its intent revision: "+result.Body.String())
	require.EqualValues(t, 1, manager.creates.Load())
}

func TestAssistantBindingRuntimeCannotSelectOrReadHumanBinding(t *testing.T) {
	s, _, task := newRuntime(t)
	router, token, runID := assistantRuntimeCaller(t, s, task)
	path := "/api/v1/orchestration/assistant"
	require.Equal(t, 403, runtimeRequest(t, router, "GET", path, token, runID, nil).Code)
	require.Equal(t, 403, runtimeRequest(t, router, "PUT", path, token, runID, map[string]any{"orchestrator_id": "chief", "expected_version": 1}).Code)
	require.Equal(t, 409, runtimeRequest(t, assistantRouter(s, "foreign"), "PUT", path, "", "", map[string]any{"orchestrator_id": "chief"}).Code)
}

func assistantDeliveryRequest(t *testing.T, s *Service, db *sqlx.DB, task string, router *gin.Engine, token, runID, operation string) map[string]any {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{ID: "delivery-source", TaskID: task, AuthorType: "user", AuthorID: "owner", Source: "user", Body: "Implement a synthetic sample"}))
	b, err := s.Repo.AssistantForConversation(ctx, task)
	require.NoError(t, err)
	o := &models.Objective{BindingID: b.ID, WorkspaceID: b.WorkspaceID, SourceCommentID: "delivery-source", Title: "Synthetic sample", Mode: "execute", Status: "active", Acceptance: []models.Criterion{{ID: "tested", Description: "Checks pass"}}}
	require.NoError(t, s.Repo.CreateObjective(ctx, o))
	_, err = db.Exec(`INSERT INTO tasks(id,workspace_id,title,created_at,updated_at) VALUES('created-task','ws','Synthetic sample',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP) ON CONFLICT(id) DO NOTHING`)
	require.NoError(t, err)
	response := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/context/"+o.ID+"?profile_id=personal", token, runID, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var packet models.ContextPacket
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &packet))
	return map[string]any{"title": "Synthetic sample", "execution_mode": "execute", "objective_id": o.ID, "context_ref": packet.ID, "operation_id": operation, "expected_intent_revision": 0}
}
