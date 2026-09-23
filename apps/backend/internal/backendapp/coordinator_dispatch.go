package backendapp

import (
	"context"
	"database/sql"
	"errors"

	orchstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// coordinatorDispatchGuard admits every native dispatch except one on a
// coordinator conversation, which must come from its broker-pinned session
// under the conversation's claimed run.
func coordinatorDispatchGuard(s *orchestrationruntime.Service, owners *orchstore.Repository) executor.DispatchGuard {
	return func(ctx context.Context, task *models.Task, session *models.TaskSession, _ string) error {
		if owners == nil {
			return nil
		}
		_, _, err := owners.ConversationOwner(ctx, task.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if s == nil {
			return orchestrationruntime.ErrOrchestrationDisabled
		}
		return s.CheckCoordinatorSession(ctx, task.ID, session)
	}
}

type dispatchGuardSetter interface {
	SetDispatchGuard(executor.DispatchGuard)
}

// wireCoordinatorDispatch installs the coordinator conversation guard on every
// native dispatch path.
func wireCoordinatorDispatch(orch dispatchGuardSetter, s *orchestrationruntime.Service, owners *orchstore.Repository) {
	orch.SetDispatchGuard(coordinatorDispatchGuard(s, owners))
}
