package sqlite

import (
	"context"
	"time"

	runmodels "github.com/kandev/kandev/internal/runs/models"
)

func (r *Repository) InterruptedRuns(ctx context.Context) ([]*runmodels.Run, error) {
	rows := []*runmodels.Run{}
	err := r.ro.SelectContext(ctx, &rows, `SELECT r.* FROM runs r JOIN workspace_orchestrators o ON o.agent_id=r.agent_profile_id WHERE r.status='claimed'`)
	return rows, err
}

// UnboundClaimedRuns lists coordinator runs claimed before the cutoff that
// never bound a session.
func (r *Repository) UnboundClaimedRuns(ctx context.Context, claimedBefore time.Time) ([]*runmodels.Run, error) {
	rows := []*runmodels.Run{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT r.* FROM runs r JOIN workspace_orchestrators o ON o.agent_id=r.agent_profile_id
 WHERE r.status='claimed' AND COALESCE(r.session_id,'')='' AND r.claimed_at < ?`), claimedBefore)
	return rows, err
}

func (r *Repository) RegisteredProfileIDs(ctx context.Context) ([]string, error) {
	ids := []string{}
	err := r.ro.SelectContext(ctx, &ids, `SELECT agent_id FROM workspace_orchestrators ORDER BY agent_id`)
	return ids, err
}
