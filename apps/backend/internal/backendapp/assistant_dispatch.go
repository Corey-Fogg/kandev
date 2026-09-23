package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	orchstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/dispatchcontext"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func assistantDispatchGuard(s *orchestrationruntime.Service, owners *orchstore.Repository) executor.DispatchGuard {
	return func(ctx context.Context, task *models.Task, session *models.TaskSession, profile string) error {
		if err := checkMaintenanceDispatch(ctx, owners, task); err != nil {
			return err
		}
		if coordinator, err := checkCoordinatorDispatch(ctx, s, owners, task.ID, session); coordinator || err != nil {
			return err
		}
		baseline, _ := task.Metadata[dispatchcontext.MetadataKey].(string)
		ref, explicit := dispatchcontext.Reference(ctx)
		if baseline == "" && ref == "" {
			return nil
		}
		if !explicit {
			ref = baseline
		}
		if s == nil || ref == "" {
			return dispatchcontext.ErrStale
		}
		// Launch/resume may also resend the task description. Do not allow a
		// fresh follow-up to launder a stale initial handoff.
		if baseline != "" && baseline != ref {
			if err := s.ValidateDispatchContext(ctx, baseline, task, profile); err != nil {
				return fmt.Errorf("%w: %s", dispatchcontext.ErrStale, err)
			}
		}
		if err := s.ValidateDispatchContext(ctx, ref, task, profile); err != nil {
			return fmt.Errorf("%w: %s", dispatchcontext.ErrStale, err)
		}
		return nil
	}
}

func checkMaintenanceDispatch(ctx context.Context, owners *orchstore.Repository, task *models.Task) error {
	if id, _ := task.Metadata["orchestration_maintenance_candidate"].(string); id != "" {
		return fmt.Errorf("maintenance tasks require the closed Assistant repair controls")
	}
	if owners == nil {
		return nil
	}
	maintenance, err := owners.IsMaintenanceTask(ctx, task.ID)
	if err != nil {
		return err
	}
	if maintenance {
		return fmt.Errorf("maintenance tasks require the closed Assistant repair controls")
	}
	return nil
}

// checkCoordinatorDispatch reports whether taskID is a coordinator
// conversation and, if so, admits only its broker-pinned session.
func checkCoordinatorDispatch(ctx context.Context, s *orchestrationruntime.Service, owners *orchstore.Repository, taskID string, session *models.TaskSession) (bool, error) {
	if owners == nil {
		return false, nil
	}
	_, _, err := owners.ConversationOwner(ctx, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if s == nil {
		return true, orchestrationruntime.ErrOrchestrationDisabled
	}
	return true, s.CheckCoordinatorSession(ctx, taskID, session)
}

// Wire even with orchestration disabled: disabling a feature is not permission
// to execute previously delegated work without its context policy.
func wireAssistantDispatch(orch *orchestrator.Service, s *orchestrationruntime.Service, tasks *taskservice.Service, owners *orchstore.Repository) {
	orch.SetDispatchGuard(assistantDispatchGuard(s, owners), func(ctx context.Context, id string) (string, error) {
		task, err := tasks.GetTask(ctx, id)
		if err != nil {
			return "", err
		}
		ref, _ := task.Metadata[dispatchcontext.MetadataKey].(string)
		return ref, nil
	})
}
