package orchestrator

import "github.com/kandev/kandev/internal/orchestrator/executor"

// SetDispatchGuard wires optional application policy without coupling the core
// execution pipeline to orchestration persistence.
func (s *Service) SetDispatchGuard(guard executor.DispatchGuard) {
	s.executor.SetDispatchGuard(guard)
}
