package service

import (
	"context"

	"github.com/kandev/kandev/internal/office/models"
)

// The core run dispatcher (internal/runs/dispatcher) owns the claim loop in
// production. These hooks keep the Office work the in-package tick does
// around that loop: routing backoff before it, claim accounting and
// processing for each run, and the checkout backstop after it.

// PrepareDispatch lifts Office routing backoff before the core claim loop.
func (si *SchedulerIntegration) PrepareDispatch(ctx context.Context) { si.liftParkedRoutingRuns(ctx) }

// ProcessRun executes an Office-owned run already claimed by the core dispatcher.
func (si *SchedulerIntegration) ProcessRun(ctx context.Context, run *models.Run) (bool, error) {
	si.svc.recordRunClaimed(ctx, run)
	si.processRun(ctx, run)
	return true, nil
}

// FinishDispatch reaps task checkouts that no run released, after the core
// claim loop. The dispatcher recovers stale claimed runs itself.
func (si *SchedulerIntegration) FinishDispatch(ctx context.Context) { si.reapStaleCheckouts(ctx) }
