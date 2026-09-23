package runtime

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateTaskRejectsLongTitle(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &fakeTaskManager{}
	s.Manager = manager
	router, token, runID := workspaceControlCaller(t, s, task)
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, map[string]any{
		"title": strings.Repeat("Qualys inspector ", 5),
	})
	require.Equal(t, 422, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "60 characters or fewer")
	require.Zero(t, manager.creates.Load())
}
