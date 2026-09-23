package runtime

import (
	"context"
	"sync/atomic"
	"testing"

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

func TestCoordinatorCreateIgnoresRetainedBinding(t *testing.T) {
	s, db, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	bindTestAssistant(t, s, db, "owner", "chief", task)
	router, token, runID := workspaceControlCaller(t, s, task)
	result := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, map[string]any{"title": "Plain delegation"})
	require.Equal(t, 201, result.Code, result.Body.String())
	require.EqualValues(t, 1, manager.creates.Load())
	result = runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, map[string]any{"title": "Missing intent", "operation_id": "no-intent"})
	require.Equal(t, 422, result.Code, "a ledgered operation still requires its intent revision: "+result.Body.String())
	require.EqualValues(t, 1, manager.creates.Load())
}
