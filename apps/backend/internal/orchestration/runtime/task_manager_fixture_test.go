package runtime

import (
	"context"
	"sync/atomic"

	"github.com/kandev/kandev/internal/orchestration/models"
)

type fakeTaskManager struct {
	creates  atomic.Int64
	failure  error
	lastSpec models.WorkspaceTaskSpec

	directory models.WorkspaceDirectory
}

func (m *fakeTaskManager) CreateWorkspaceTask(_ context.Context, spec models.WorkspaceTaskSpec) (string, error) {
	m.creates.Add(1)
	m.lastSpec = spec
	return "created-task", m.failure
}

func (m *fakeTaskManager) ManageWorkspaceTask(context.Context, models.WorkspaceTaskCommand) error {
	return nil
}
func (m *fakeTaskManager) WorkspaceTaskDetails(context.Context, string, string) (any, error) {
	return nil, nil
}
func (m *fakeTaskManager) WorkspaceCatalog(context.Context, string, bool) (any, error) {
	return nil, nil
}
func (m *fakeTaskManager) WorkspaceDirectory(context.Context, string) (models.WorkspaceDirectory, error) {
	return m.directory, nil
}
func (m *fakeTaskManager) WorkspaceTaskSummaries(context.Context, string, int, int) ([]models.WorkspaceTaskSummary, bool, error) {
	return []models.WorkspaceTaskSummary{}, false, nil
}
