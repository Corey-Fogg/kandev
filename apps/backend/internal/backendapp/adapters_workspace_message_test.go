package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkspaceMessageTargetsLatestSessionByDefault(t *testing.T) {
	adapter, _ := newOfficeTaskAdapterHarness(t)
	adapter.orch = &orchestrator.Service{}
	ctx := context.Background()
	task := &models.Task{ID: "message-default", WorkspaceID: "ws-1", Title: "Review"}
	require.NoError(t, adapter.taskRepo.CreateTask(ctx, task))
	_, err := adapter.messageSessionID(ctx, task.ID, "")
	require.ErrorContains(t, err, "use start", "a task that never ran has no session to message")

	now := time.Now().UTC()
	for i, id := range []string{"older", "newer"} {
		session := &models.TaskSession{ID: id, TaskID: task.ID, AgentProfileID: "personal", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now.Add(time.Duration(i) * time.Minute)}
		require.NoError(t, adapter.taskRepo.CreateTaskSession(ctx, session))
	}
	id, err := adapter.messageSessionID(ctx, task.ID, "")
	require.NoError(t, err)
	require.Equal(t, "newer", id)
	id, err = adapter.messageSessionID(ctx, task.ID, "older")
	require.NoError(t, err)
	require.Equal(t, "older", id)
}
