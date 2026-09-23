package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	runmodels "github.com/kandev/kandev/internal/runs/models"
)

// AbsorbQueuedRuns folds the payloads of an agent's other queued, unscheduled
// runs with the given reason into its claimed run and cancels them, in one
// transaction. merge receives the claimed run's payload followed by the
// queued payloads in request order and returns the claimed run's new payload.
// It returns the payload the claimed run now carries.
func (r *Repository) AbsorbQueuedRuns(ctx context.Context, runID, agentID, reason, cancelReason string, merge func(string, []string) (string, error)) (string, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var own string
	if err := tx.GetContext(ctx, &own, tx.Rebind(`SELECT payload FROM runs WHERE id=? AND status='claimed'`), runID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("run is no longer claimed")
		}
		return "", err
	}
	var queued []struct {
		ID      string `db:"id"`
		Payload string `db:"payload"`
	}
	if err := tx.SelectContext(ctx, &queued, tx.Rebind(`SELECT id, payload FROM runs
		WHERE agent_profile_id=? AND reason=? AND status='queued' AND scheduled_retry_at IS NULL AND id<>?
		ORDER BY requested_at, id`), agentID, reason, runID); err != nil {
		return "", err
	}
	if len(queued) == 0 {
		return own, nil
	}
	ids := make([]string, 0, len(queued))
	payloads := make([]string, 0, len(queued))
	for _, row := range queued {
		ids = append(ids, row.ID)
		payloads = append(payloads, row.Payload)
	}
	merged, err := merge(own, payloads)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE runs SET payload=? WHERE id=? AND status='claimed'`), merged, runID); err != nil {
		return "", err
	}
	query, args, err := sqlx.In(`UPDATE runs SET status='cancelled', cancel_reason=?, finished_at=? WHERE status='queued' AND id IN (?)`, cancelReason, time.Now().UTC(), ids)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(query), args...); err != nil {
		return "", err
	}
	return merged, tx.Commit()
}

// SetClaimedRunPayload rewrites a claimed run's payload. It fails when the run
// is no longer claimed, so a settled run is never rewritten.
func (r *Repository) SetClaimedRunPayload(ctx context.Context, runID, payload string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE runs SET payload=? WHERE id=? AND status='claimed'`), payload, runID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n == 0 {
		return errors.Join(errors.New("run is no longer claimed"), err)
	}
	return nil
}

// RunByIdempotencyKey returns the run queued under key.
func (r *Repository) RunByIdempotencyKey(ctx context.Context, key string) (*runmodels.Run, error) {
	var run runmodels.Run
	err := r.ro.GetContext(ctx, &run, r.ro.Rebind(`SELECT * FROM runs WHERE idempotency_key=?`), key)
	if err != nil {
		return nil, err
	}
	return &run, nil
}
