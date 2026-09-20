package service

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/archivecascade"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// StartSessionReconciliationLoop starts the background goroutine that
// periodically reconciles task sessions that no request path owns anymore.
// Each tick runs two passes:
//
//  1. The archived-task pass: re-finalize archived tasks whose sessions
//     never made it to a terminal DB state (see runArchivedSessionReconciliation).
//  2. The active-task pass: detect and heal unarchived tasks holding active
//     sessions whose backing execution is gone (see runActiveSessionSweep in
//     active_session_stall.go).
//
// finalizeCancelledSessions (see service_tasks.go) already bounds its
// session-cancellation retry to a handful of fixed attempts inside
// ArchiveTask's own request: enough to ride out a brief SQLite writer-lock
// blip, but not unbounded. If SQLite's single writer stays occupied for
// longer than every attempt combined, that bounded retry gets exhausted
// after archived_at has already committed, and nothing else in the request
// path ever retries the transition. The archived task is then left with
// sessions stuck in an active DB state (CREATED/STARTING/RUNNING/
// WAITING_FOR_INPUT) forever, and event-driven clients that key their
// "is running" indicators off session.state_changed never learn the
// sessions actually stopped.
//
// The archived pass is the eventual-consistency backstop for that residual
// gap, following the exact periodic-sweep shape StartAutoArchiveLoop already
// uses: on a fixed interval, list every archived task that still has an
// active session, and re-invoke finalizeCancelledSessions for each. Calling
// finalizeCancelledSessions again is always safe — its underlying
// CancelActiveTaskSessionsByTaskID UPDATE only matches sessions still in an
// active state, so a task that was already fully reconciled (by ArchiveTask
// itself or by an earlier sweep pass) simply yields an empty cancelledSessions
// slice and is a no-op. As long as the process keeps running, a later pass
// retries once the SQLite contention that exhausted the in-line retry has
// cleared — unlike the fixed 3-attempt budget, this sweep never gives up.
func (s *Service) StartSessionReconciliationLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.runArchivedSessionReconciliation(ctx)
				s.runActiveSessionSweep(ctx, now)
			}
		}
	}()
	s.logger.Info("session reconciliation loop started (every 1 minute)")
}

func (s *Service) runArchivedSessionReconciliation(ctx context.Context) {
	deadline := archivecascade.ArchiveDeadline(ctx)
	reconcileCtx, cancel := archivecascade.ContinuationContextUntil(ctx, deadline)
	defer cancel()
	taskIDs, err := s.tasks.ListArchivedTasksWithActiveSessions(reconcileCtx)
	if err != nil {
		s.logger.Error("archived-session reconciliation: failed to list candidates", zap.Error(err))
		return
	}
	if len(taskIDs) == 0 {
		return
	}

	s.logger.Info("archived-session reconciliation: found candidates", zap.Int("count", len(taskIDs)))
	for _, taskID := range taskIDs {
		activeSessions, err := s.sessions.ListActiveTaskSessionsByTaskID(reconcileCtx, taskID)
		if err != nil {
			s.logger.Warn("archived-session reconciliation: failed to list active sessions",
				zap.String("task_id", taskID),
				zap.Error(err))
			continue
		}
		if len(activeSessions) == 0 {
			// Already reconciled by a concurrent pass or by ArchiveTask itself
			// between the candidate list query and this read.
			continue
		}
		s.finalizeCancelledSessions(reconcileCtx, taskID, activeSessions, deadline, models.SessionArchiveCancelReason)
	}
}
