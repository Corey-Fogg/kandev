package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestOrchestratorBrokerForwardsOnlyNamedOperations(t *testing.T) {
	calls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "Bearer synthetic", r.Header.Get("Authorization"))
		require.Equal(t, "/api/v1/orchestration/runtime/tasks/target/manage", r.URL.Path)
		require.Equal(t, "run", r.Header.Get("X-Kandev-Run-Id"))
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":"operation_outcome_unknown"}`))
	}))
	defer backend.Close()
	c := &kandevClient{apiURL: backend.URL, apiKey: "synthetic", runID: "run", http: backend.Client()}
	s := newOrchestratorMCP(c)
	require.Len(t, s.ListTools(), len(models.WorkspaceBrokerTools()))
	require.Nil(t, s.GetTool("shell"))
	require.Nil(t, s.GetTool("invoke_plugin"))
	definition := models.WorkspaceBrokerTool{Name: "manage_task", Method: http.MethodPost, Path: "/runtime/tasks/:id/manage"}
	result, err := callOrchestratorBroker(c, definition, map[string]any{"id": "target", "request": map[string]any{"action": "start"}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Equal(t, 1, calls, "unknown delivery must not be retried")
	for _, id := range []string{"../settings", "../..", "%2f", "x?admin=1", "x/y"} {
		result, err = callOrchestratorBroker(c, definition, map[string]any{"id": id})
		require.NoError(t, err)
		require.True(t, result.IsError)
	}
	require.Equal(t, 1, calls)
}

func TestOrchestratorBrokerAdvertisesWorkspaceTools(t *testing.T) {
	s := newOrchestratorMCP(&kandevClient{})
	for _, name := range []string{"manage_workspace", "workspace", "workspace_tasks", "task_details", "task_content", "task_permissions", "comments", "capabilities", "memory", "remember", "forget", "create_task", "manage_task", "task_status", "comment"} {
		require.NotNil(t, s.GetTool(name), name)
	}
	for _, name := range []string{"create_objective", "objectives", "maintenance", "workspace_links", "context", "attention", "improvements"} {
		require.Nil(t, s.GetTool(name), name)
	}
	for _, action := range []string{"edit", "move", "archive", "delete", "assign", "start", "stop", "message", "session_mode", "resolve_permission", "answer_question"} {
		require.Contains(t, s.GetTool("manage_task").Tool.Description, action)
	}
	for _, resource := range []string{"workflow", "step", "repository", "configuration"} {
		require.Contains(t, s.GetTool("manage_workspace").Tool.Description, resource)
	}
	for _, tool := range models.WorkspaceBrokerTools() {
		require.NotContains(t, tool.Description, "operation_id", tool.Name)
		require.NotContains(t, tool.Description, "objective", tool.Name)
	}
}

func TestOrchestratorBrokerReportsEmptyHTTPFailure(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) }))
	defer backend.Close()
	result, err := callOrchestratorBroker(&kandevClient{apiURL: backend.URL, http: backend.Client()}, models.WorkspaceBrokerTool{Method: http.MethodGet, Path: "/runtime/tasks"}, nil)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content, mcp.TextContent{Type: "text", Text: "Kandev returned HTTP 404 (Not Found)"})
}

func TestKandevCLIDispatchesOrchestratorBroker(t *testing.T) {
	t.Setenv("KANDEV_API_URL", "")
	t.Setenv("KANDEV_API_KEY", "")
	require.Equal(t, 1, runKandevCLI([]string{"orchestrator-mcp"}), "the broker refuses to start without runtime credentials")
	require.Equal(t, 1, runKandevCLI([]string{"assistant-mcp"}), "the retired subcommand is unknown")
}
