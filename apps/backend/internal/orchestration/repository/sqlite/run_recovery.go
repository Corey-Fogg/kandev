package sqlite

import (
	"context"
	"slices"
	"sync"
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

// registeredCacheTTL bounds how long a registration written outside this
// repository (a restored backup, for example) can go unseen.
const registeredCacheTTL = 30 * time.Second

type registeredCache struct {
	mu       sync.Mutex
	ids      []string
	loadedAt time.Time
}

// RegisteredProfileIDs lists every registered orchestrator profile. The list
// is cached; this repository's register and unregister writes refresh it.
func (r *Repository) RegisteredProfileIDs(ctx context.Context) ([]string, error) {
	c := &r.registered
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.loadedAt.IsZero() && time.Since(c.loadedAt) < registeredCacheTTL {
		return slices.Clone(c.ids), nil
	}
	ids := []string{}
	if err := r.ro.SelectContext(ctx, &ids, `SELECT agent_id FROM workspace_orchestrators ORDER BY agent_id`); err != nil {
		return nil, err
	}
	c.ids, c.loadedAt = ids, time.Now()
	return slices.Clone(ids), nil
}

func (r *Repository) invalidateRegistered() {
	r.registered.mu.Lock()
	r.registered.loadedAt = time.Time{}
	r.registered.mu.Unlock()
}
