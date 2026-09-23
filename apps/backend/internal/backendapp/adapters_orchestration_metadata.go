package backendapp

import (
	"context"
	"fmt"

	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
)

type orchestrationMetadataRepo interface {
	SetTaskMetadataKeyIfNotArchived(ctx context.Context, taskID, key string, value interface{}) (bool, error)
}

type orchestrationMetadataTasks interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
	PublishTaskUpdated(ctx context.Context, task *models.Task, fields ...string)
}

// orchestrationTaskMetadata writes the task metadata keys orchestration owns
// and publishes task.updated so the Coordinator view follows.
type orchestrationTaskMetadata struct {
	repo  orchestrationMetadataRepo
	tasks orchestrationMetadataTasks
}

// SetTaskMetadata writes one orchestration-owned key of a live task. It
// reports false when the task is archived or missing.
func (m orchestrationTaskMetadata) SetTaskMetadata(ctx context.Context, taskID, key string, value any) (bool, error) {
	if key != shared.MetaTaskGoal && key != shared.MetaTaskStall {
		return false, fmt.Errorf("task metadata key %q is not owned by orchestration", key)
	}
	changed, err := m.repo.SetTaskMetadataKeyIfNotArchived(ctx, taskID, key, value)
	if err != nil || !changed {
		return changed, err
	}
	task, err := m.tasks.GetTask(ctx, taskID)
	if err != nil {
		return true, err
	}
	m.tasks.PublishTaskUpdated(ctx, task)
	return true, nil
}
