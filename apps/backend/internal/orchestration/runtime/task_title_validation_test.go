package runtime

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateTaskRejectsLongTitleWithoutUnknownOutcome(t *testing.T) {
	s, db, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, map[string]any{
		"title": strings.Repeat("Qualys inspector ", 5), "operation_id": "long-title", "expected_intent_revision": 0,
	})
	require.Equal(t, 422, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "60 characters or fewer")
	require.Zero(t, manager.creates.Load())
	var recorded int
	require.NoError(t, db.Get(&recorded, "SELECT count(*) FROM orchestration_operations WHERE operation_id='long-title'"))
	require.Zero(t, recorded, "a definite rejection must not leave an unknown operation behind")
}
