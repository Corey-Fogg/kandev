package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestration/models"
)

func TestOrchestratorBrokerPublishesTypedSchemas(t *testing.T) {
	s := newOrchestratorMCP(&kandevClient{})
	manage := s.GetTool("manage_task").Tool.InputSchema
	require.NotContains(t, manage.Required, "id", "a batched tool accepts ids in place of id")
	require.Contains(t, manage.Properties, "ids")
	request := manage.Properties["request"].(map[string]any)
	require.Equal(t, []string{"action"}, request["required"])
	action := request["properties"].(map[string]any)["action"].(map[string]any)
	require.Contains(t, action["enum"], "answer_question")
	create := s.GetTool("create_task").Tool.InputSchema.Properties["request"].(map[string]any)
	require.Equal(t, []string{"title"}, create["required"])
	details := s.GetTool("task_details").Tool.InputSchema
	require.Contains(t, details.Required, "id")
	require.Contains(t, details.Properties["query"].(map[string]any)["properties"], "include_result")
}

func TestOrchestratorBrokerBatchReportsEachTask(t *testing.T) {
	var paths []string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.Contains(r.URL.Path, "/locked/") {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"task must belong to this workspace"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer backend.Close()
	c := &kandevClient{apiURL: backend.URL, runID: "run", http: backend.Client()}
	definition := models.WorkspaceBrokerTool{Name: "task_status", Method: http.MethodPost, Path: "/runtime/tasks/:id/status", Batch: true}
	result, err := callOrchestratorBatch(c, definition, []any{"first", "locked", "../x"}, map[string]any{"request": map[string]any{"status": "done"}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, []string{"/api/v1/orchestration/runtime/tasks/first/status", "/api/v1/orchestration/runtime/tasks/locked/status"}, paths, "an unsafe id is refused before any request")
	var body struct {
		Results []map[string]any `json:"results"`
		Failed  int              `json:"failed"`
	}
	require.NoError(t, json.Unmarshal([]byte(brokerResultText(result)), &body))
	require.Equal(t, 2, body.Failed)
	require.Equal(t, true, body.Results[0]["ok"])
	require.Contains(t, body.Results[1]["error"], "task must belong to this workspace")
	require.Equal(t, "invalid resource id", body.Results[2]["error"])
}

func TestOrchestratorBrokerBatchRefusesPerTaskActions(t *testing.T) {
	c := &kandevClient{apiURL: "http://127.0.0.1:1", http: http.DefaultClient}
	definition := models.WorkspaceBrokerTool{Name: "manage_task", Method: http.MethodPost, Path: "/runtime/tasks/:id/manage", Batch: true}
	for _, request := range []map[string]any{{"action": "message", "prompt": "Continue"}, {"action": "delete"}} {
		result, err := callOrchestratorBatch(c, definition, []any{"first"}, map[string]any{"request": request})
		require.NoError(t, err)
		require.True(t, result.IsError)
	}
	ids := make([]any, models.BrokerBatchLimit+1)
	result, err := callOrchestratorBatch(c, definition, ids, map[string]any{"request": map[string]any{"action": "move"}})
	require.NoError(t, err)
	require.True(t, result.IsError)
}
