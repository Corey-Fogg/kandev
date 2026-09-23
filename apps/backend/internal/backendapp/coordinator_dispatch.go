package backendapp

import (
	"context"
	"database/sql"
	"errors"

	orchstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// coordinatorDispatchGuard admits every native dispatch except one on a
// coordinator conversation, which must come from its broker-pinned session
// under the conversation's claimed run.
func coordinatorDispatchGuard(s *orchestrationruntime.Service, owners *orchstore.Repository) executor.DispatchGuard {
	return func(ctx context.Context, target executor.DispatchTarget) error {
		if owners == nil {
			return nil
		}
		// One read decides an ordinary dispatch; only a coordinator
		// conversation loads its session.
		_, _, err := owners.ConversationOwner(ctx, target.TaskID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if s == nil || !s.Enabled {
			return orchestrationruntime.ErrOrchestrationDisabled
		}
		session, err := target.Session()
		if err != nil {
			return err
		}
		return s.CheckCoordinatorSession(ctx, target.TaskID, session)
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
