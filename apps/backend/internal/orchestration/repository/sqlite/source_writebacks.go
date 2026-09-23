package sqlite

import (
	"context"
	"time"
)

// Source write-back ledger statuses.
const (
	WriteBackClaimed = "claimed"
	WriteBackPosted  = "posted"
	WriteBackFailed  = "failed"
	WriteBackSkipped = "skipped"
)

// migrateSourceWriteBacks creates the per-task ledger that makes automatic
// tracker write-back happen at most once per state transition.
func (r *Repository) migrateSourceWriteBacks() error {
	_, err := r.db.Exec(renderSchema(r.db.DriverName(), `CREATE TABLE IF NOT EXISTS orchestration_source_writebacks (
		task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
		last_state TEXT NOT NULL,
		episode INTEGER NOT NULL DEFAULT 1,
		status TEXT NOT NULL DEFAULT '',
		commented {{boolean}} NOT NULL DEFAULT FALSE,
		moved_state TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMP NOT NULL)`))
	return err
}

// ObserveTaskState records a delegated task's state. Only a change into a
// new state starts an episode; a redelivered state changes nothing. It
// claims the new episode's write-back when reportable, so a write-back is
// attempted at most once per transition.
//
// The first observation of a task is a baseline: it is claimed only when
// transitioned says the event itself changed the task's state. A task that
// was already in review or complete before the ledger saw it (an upgrade, or
// a transition made while its coordinator was paused) therefore posts
// nothing when an unrelated event, such as a reorder, arrives.
func (r *Repository) ObserveTaskState(ctx context.Context, taskID, state string, reportable, transitioned bool) (int64, bool, error) {
	now := time.Now().UTC()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback() }()
	// Write first so SQLite takes its write lock before the read below.
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_source_writebacks(task_id,last_state,episode,status,updated_at)
		VALUES(?,?,1,'',?) ON CONFLICT(task_id) DO NOTHING`), taskID, state, now)
	if err != nil {
		return 0, false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	var row struct {
		LastState string `db:"last_state"`
		Episode   int64  `db:"episode"`
	}
	if err := tx.GetContext(ctx, &row, tx.Rebind(`SELECT last_state,episode FROM orchestration_source_writebacks WHERE task_id=?`), taskID); err != nil {
		return 0, false, err
	}
	if inserted == 0 {
		if row.LastState == state {
			return row.Episode, false, tx.Commit()
		}
		result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_source_writebacks
			SET last_state=?,episode=episode+1,status='',commented=?,moved_state='',error='',updated_at=?
			WHERE task_id=? AND last_state=?`), state, false, now, taskID, row.LastState)
		if err != nil {
			return 0, false, err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return row.Episode, false, err
		}
		row.Episode++
	}
	claimed := false
	if reportable && (inserted == 0 || transitioned) {
		result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_source_writebacks SET status=?,updated_at=? WHERE task_id=? AND episode=? AND status=''`),
			WriteBackClaimed, now, taskID, row.Episode)
		if err != nil {
			return 0, false, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, false, err
		}
		claimed = n == 1
	}
	return row.Episode, claimed, tx.Commit()
}

// FinishSourceWriteBack records the outcome of an episode's write-back.
func (r *Repository) FinishSourceWriteBack(ctx context.Context, taskID string, episode int64, status string, commented bool, movedState, errText string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE orchestration_source_writebacks SET status=?,commented=?,moved_state=?,error=?,updated_at=? WHERE task_id=? AND episode=?`),
		status, commented, movedState, errText, time.Now().UTC(), taskID, episode)
	return err
}

// SourceWriteBack is one task's write-back ledger row.
type SourceWriteBack struct {
	TaskID     string `db:"task_id"`
	LastState  string `db:"last_state"`
	Episode    int64  `db:"episode"`
	Status     string `db:"status"`
	Commented  bool   `db:"commented"`
	MovedState string `db:"moved_state"`
	Error      string `db:"error"`
}

// GetSourceWriteBack reads a task's ledger row.
func (r *Repository) GetSourceWriteBack(ctx context.Context, taskID string) (*SourceWriteBack, error) {
	var row SourceWriteBack
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind(`SELECT task_id,last_state,episode,status,commented,moved_state,error FROM orchestration_source_writebacks WHERE task_id=?`), taskID)
	return &row, err
}
