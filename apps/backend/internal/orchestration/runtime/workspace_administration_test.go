package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type workspaceAdministratorFake struct {
	*fakeTaskManager
	workspace string
	calls     int
}

func (m *workspaceAdministratorFake) ManageWorkspace(_ context.Context, workspace string, _ json.RawMessage) (any, error) {
	m.workspace = workspace
	m.calls++
	return map[string]string{"name": "Synthetic workspace"}, nil
}

func TestWorkspaceAdministrationRequiresCurrentSignedScope(t *testing.T) {
	s, _, task := newRuntime(t)
	m := &workspaceAdministratorFake{fakeTaskManager: &fakeTaskManager{}}
	s.Manager = m
	router, token, run := workspaceControlCaller(t, s, task)
	body := map[string]any{"resource": "workspace", "action": "update", "configuration": map[string]string{"name": "Synthetic workspace"}}
	path := "/api/v1/orchestration/runtime/workspace/manage"
	response := runtimeRequest(t, router, "POST", path, token, run, body)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Equal(t, "ws", m.workspace)
	require.Equal(t, 403, runtimeRequest(t, router, "POST", path+"?workspace_id=foreign", token, run, body).Code)
	_, finishErr := s.Runs.FinishRun(context.Background(), run, "finished", nil)
	require.NoError(t, finishErr)
	require.Equal(t, 403, runtimeRequest(t, router, "POST", path, token, run, body).Code)
	require.Equal(t, 1, m.calls)
}
