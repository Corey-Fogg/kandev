package sqlite

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	kdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// DelegatedTask is one task a coordinator delegated, as outcome metrics read it.
type DelegatedTask struct {
	ID        string    `db:"id"`
	State     string    `db:"state"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// DelegatedTasksSince lists up to limit tasks the coordinator delegated in the
// workspace since the given time, newest first.
func (r *Repository) DelegatedTasksSince(ctx context.Context, workspaceID, agentID string, since time.Time, limit int) ([]DelegatedTask, error) {
	tasks := []DelegatedTask{}
	query := "SELECT id, state, created_at, updated_at FROM tasks WHERE workspace_id=? AND " +
		dialect.JSONExtract(r.ro.DriverName(), "metadata", "orchestration_chief_id") + "=? AND created_at>=? ORDER BY created_at DESC, id LIMIT ?"
	err := r.ro.SelectContext(ctx, &tasks, r.ro.Rebind(query), workspaceID, agentID, since.UTC(), limit)
	return tasks, err
}

// FirstMergedAt returns each task's earliest pull request merge time. It
// reports false when the pull request store is not installed.
func (r *Repository) FirstMergedAt(ctx context.Context, taskIDs []string) (map[string]time.Time, bool, error) {
	merged := map[string]time.Time{}
	if len(taskIDs) == 0 {
		return merged, true, nil
	}
	query, args, err := sqlx.In("SELECT task_id, merged_at FROM github_task_prs WHERE merged_at IS NOT NULL AND task_id IN (?)", taskIDs)
	if err != nil {
		return nil, false, err
	}
	var rows []struct {
		TaskID   string    `db:"task_id"`
		MergedAt time.Time `db:"merged_at"`
	}
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), args...); err != nil {
		if kdb.IsMissingTableError(err) {
			return merged, false, nil
		}
		return nil, false, err
	}
	for _, row := range rows {
		if first, ok := merged[row.TaskID]; !ok || row.MergedAt.Before(first) {
			merged[row.TaskID] = row.MergedAt
		}
	}
	return merged, true, nil
}

// UsageCost sums the priced cost of the tasks' usage since the given time and
// counts the events that carried no price.
func (r *Repository) UsageCost(ctx context.Context, taskIDs []string, since time.Time) (int64, int, error) {
	if len(taskIDs) == 0 {
		return 0, 0, nil
	}
	query, args, err := sqlx.In(`SELECT COALESCE(SUM(cost_subcents),0) AS cost, COALESCE(SUM(CASE WHEN cost_source='unpriced' THEN 1 ELSE 0 END),0) AS unpriced
		FROM task_usage_events WHERE occurred_at>=? AND task_id IN (?)`, since.UTC(), taskIDs)
	if err != nil {
		return 0, 0, err
	}
	var row struct {
		Cost     int64 `db:"cost"`
		Unpriced int   `db:"unpriced"`
	}
	err = r.ro.GetContext(ctx, &row, r.ro.Rebind(query), args...)
	return row.Cost, row.Unpriced, err
}
