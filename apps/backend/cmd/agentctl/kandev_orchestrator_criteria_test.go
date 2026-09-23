package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestration/models"
)

func TestOrchestratorBrokerAdvertisesCriteriaAndProposals(t *testing.T) {
	s := newOrchestratorMCP(&kandevClient{})
	require.NotNil(t, s.GetTool("task_proposals"))
	manage, err := json.Marshal(s.GetTool("manage_task").Tool.InputSchema)
	require.NoError(t, err)
	for _, want := range []string{`"acceptance_criteria"`, `"criteria"`, `"set_criteria"`, `"verify_criteria"`, `"evidence"`} {
		require.Contains(t, string(manage), want)
	}
	create, err := json.Marshal(s.GetTool("create_task").Tool.InputSchema)
	require.NoError(t, err)
	require.Contains(t, string(create), `"acceptance_criteria"`)
	require.Contains(t, s.GetTool("task_status").Tool.Description, "verify_criteria")
}

func TestOrchestratorBrokerRefusesBatchedCriteriaActions(t *testing.T) {
	calls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++ }))
	defer backend.Close()
	client := &kandevClient{apiURL: backend.URL, http: backend.Client()}
	definition := models.WorkspaceBrokerTool{Name: "manage_task", Method: http.MethodPost, Path: "/runtime/tasks/:id/manage", Batch: true}
	for _, action := range []string{"verify_criteria", "set_criteria"} {
		result, err := callOrchestratorBatch(client, definition, []any{"a", "b"}, map[string]any{"request": map[string]any{"action": action}})
		require.NoError(t, err)
		require.True(t, result.IsError, action)
	}
	require.Zero(t, calls)
}
