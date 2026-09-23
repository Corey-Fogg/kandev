package runtime

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssistantRoutingAnswerAndInspectCannotCreateDeliveryTasks(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	for _, mode := range []string{"answer", "inspect", "unknown"} {
		request := map[string]any{"title": "No delivery", "execution_mode": mode, "operation_id": mode, "expected_intent_revision": 0}
		result := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, request)
		require.Equal(t, 422, result.Code, result.Body.String())
	}
	require.Zero(t, manager.creates.Load())
}
