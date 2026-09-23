package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// migrateProposals creates the durable store of create_task proposals.
func (r *Repository) migrateProposals() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS orchestration_task_proposals (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL REFERENCES workspace_orchestrators(agent_id) ON DELETE CASCADE,
			workspace_id TEXT NOT NULL,
			conversation_task_id TEXT NOT NULL,
			run_id TEXT NOT NULL DEFAULT '',
			request_hash TEXT NOT NULL,
			source_key TEXT NOT NULL DEFAULT '',
			spec TEXT NOT NULL,
			final_spec TEXT NOT NULL DEFAULT '',
			edited {{boolean}} NOT NULL DEFAULT FALSE,
			status TEXT NOT NULL DEFAULT 'pending',
			task_id TEXT NOT NULL DEFAULT '',
			duplicate {{boolean}} NOT NULL DEFAULT FALSE,
			dismiss_reason TEXT NOT NULL DEFAULT '',
			decided_by TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			decided_at TIMESTAMP,
			UNIQUE(agent_id, run_id, request_hash))`,
		`CREATE INDEX IF NOT EXISTS idx_orchestration_task_proposals_agent
			ON orchestration_task_proposals(agent_id, status, created_at)`,
	} {
		if _, err := r.db.Exec(renderSchema(r.db.DriverName(), q)); err != nil {
			return err
		}
	}
	for _, column := range []struct{ name, definition string }{
		{"claim_token", `TEXT NOT NULL DEFAULT ''`},
		{"claimed_at", `TIMESTAMP`},
	} {
		exists, err := db.ColumnExists(r.db, "orchestration_task_proposals", column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		statement := `ALTER TABLE orchestration_task_proposals ADD COLUMN ` + column.name + ` ` + column.definition
		if _, err := r.db.Exec(statement); err != nil && !db.IsDuplicateColumnError(err) {
			return err
		}
	}
	return r.migrateUndecidedSourceIndex()
}

// migrateUndecidedSourceIndex allows one undecided proposal per source issue
// and orchestrator. A database that already holds duplicates keeps working
// without the index; CreateProposal still checks inside its transaction.
func (r *Repository) migrateUndecidedSourceIndex() error {
	var duplicated int
	if err := r.db.Get(&duplicated, `SELECT COUNT(*) FROM (SELECT agent_id,source_key FROM orchestration_task_proposals
		WHERE source_key<>'' AND status IN ('pending','approving') GROUP BY agent_id,source_key HAVING COUNT(*)>1) d`); err != nil {
		return err
	}
	if duplicated > 0 {
		logger.Default().Info("orchestration: skipping the undecided-proposal source index while duplicates exist",
			zap.Int("sources", duplicated))
		return nil
	}
	_, err := r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_orchestration_task_proposals_undecided_source
		ON orchestration_task_proposals(agent_id, source_key) WHERE source_key<>'' AND status IN ('pending','approving')`)
	return err
}

const proposalColumns = `id,agent_id,workspace_id,conversation_task_id,run_id,request_hash,source_key,spec,final_spec,edited,status,task_id,duplicate,dismiss_reason,decided_by,created_at,decided_at,claimed_at`

// CreateProposal stores a proposal and its conversation comment together.
// The comment's id is the proposal id. A replay of the same request in the
// same run returns the stored proposal with created=false, and so does an
// undecided proposal for the same source issue: the check runs inside the
// insert's transaction (and against a partial unique index), so parallel
// requests for one issue store one proposal.
func (r *Repository) CreateProposal(ctx context.Context, p *models.TaskProposal, comment *models.TaskComment) (*models.TaskProposal, bool, error) {
	spec, err := json.Marshal(p.Spec)
	if err != nil {
		return nil, false, err
	}
	p.SpecJSON = string(spec)
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	// Write first so SQLite takes its write lock before the source check.
	// DO NOTHING without a target also absorbs the undecided-source index.
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_task_proposals
		(id,agent_id,workspace_id,conversation_task_id,run_id,request_hash,source_key,spec,status,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`),
		p.ID, p.OrchestratorID, p.WorkspaceID, p.ConversationTaskID, p.RunID, p.RequestHash, p.SourceKey, p.SpecJSON, models.ProposalPending, p.CreatedAt)
	if err != nil {
		return nil, false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if n == 1 && p.SourceKey != "" {
		var other int
		if err := tx.GetContext(ctx, &other, tx.Rebind(`SELECT COUNT(*) FROM orchestration_task_proposals
			WHERE agent_id=? AND source_key=? AND status IN (?,?) AND id<>?`),
			p.OrchestratorID, p.SourceKey, models.ProposalPending, models.ProposalApproving, p.ID); err != nil {
			return nil, false, err
		}
		if other > 0 {
			n = 0
		}
	}
	if n == 0 {
		// Release the write transaction before reading the stored proposal.
		_ = tx.Rollback()
		return r.existingProposal(ctx, p)
	}
	comment.ID, comment.CreatedAt = p.ID, p.CreatedAt
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_comments(id,task_id,author_type,author_id,body,source,created_at) VALUES(?,?,?,?,?,?,?)`),
		comment.ID, comment.TaskID, comment.AuthorType, comment.AuthorID, comment.Body, comment.Source, comment.CreatedAt); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	p.Status = models.ProposalPending
	return p, true, nil
}

// existingProposal is the stored proposal that kept p from being inserted:
// the same request replayed in the same run, else the undecided proposal for
// p's source issue.
func (r *Repository) existingProposal(ctx context.Context, p *models.TaskProposal) (*models.TaskProposal, bool, error) {
	existing, err := r.proposalWhere(ctx, `agent_id=? AND run_id=? AND request_hash=?`, p.OrchestratorID, p.RunID, p.RequestHash)
	if !errors.Is(err, models.ErrProposalNotFound) || p.SourceKey == "" {
		return existing, false, err
	}
	pending, err := r.PendingProposalForSource(ctx, p.OrchestratorID, p.SourceKey)
	if err == nil && pending == nil {
		err = models.ErrProposalNotFound
	}
	return pending, false, err
}

// PendingProposalForSource returns the orchestrator's undecided proposal for
// a source issue, or nil.
func (r *Repository) PendingProposalForSource(ctx context.Context, agentID, sourceKey string) (*models.TaskProposal, error) {
	p, err := r.proposalWhere(ctx, `agent_id=? AND source_key=? AND status IN (?,?) ORDER BY created_at DESC LIMIT 1`,
		agentID, sourceKey, models.ProposalPending, models.ProposalApproving)
	if errors.Is(err, models.ErrProposalNotFound) {
		return nil, nil
	}
	return p, err
}

// GetProposal returns one of the orchestrator's proposals.
func (r *Repository) GetProposal(ctx context.Context, agentID, id string) (*models.TaskProposal, error) {
	return r.proposalWhere(ctx, `agent_id=? AND id=?`, agentID, id)
}

// ListProposals lists the orchestrator's proposals, newest first. status
// "pending" selects undecided ones; "" selects all.
func (r *Repository) ListProposals(ctx context.Context, agentID, status string, limit int) ([]models.TaskProposal, error) {
	query, args := `SELECT `+proposalColumns+` FROM orchestration_task_proposals WHERE agent_id=?`, []any{agentID}
	if status == models.ProposalPending {
		query += ` AND status IN (?,?)`
		args = append(args, models.ProposalPending, models.ProposalApproving)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows := []models.TaskProposal{}
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, err
	}
	for i := range rows {
		if err := rows[i].DecodeSpecs(); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (r *Repository) proposalWhere(ctx context.Context, where string, args ...any) (*models.TaskProposal, error) {
	var row models.TaskProposal
	err := r.db.GetContext(ctx, &row, r.db.Rebind(`SELECT `+proposalColumns+` FROM orchestration_task_proposals WHERE `+where), args...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.ErrProposalNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, row.DecodeSpecs()
}

// ClaimProposalApproval marks a pending proposal as being approved under
// token. An approval already in flight is not claimed again, so a second
// approve cannot create a task alongside it; only a claim older than
// staleBefore (an interrupted approval) can be taken over.
func (r *Repository) ClaimProposalApproval(ctx context.Context, id, token string, now, staleBefore time.Time) (bool, error) {
	return r.changed(ctx, `UPDATE orchestration_task_proposals SET status=?,claim_token=?,claimed_at=?
		WHERE id=? AND (status=? OR (status=? AND (claimed_at IS NULL OR claimed_at<?)))`,
		models.ProposalApproving, token, now.UTC(), id, models.ProposalPending, models.ProposalApproving, staleBefore.UTC())
}

// ReleaseProposalClaim returns a failed approval to pending. Only the
// holder of the claim can release it.
func (r *Repository) ReleaseProposalClaim(ctx context.Context, id, token string) error {
	_, err := r.changed(ctx, `UPDATE orchestration_task_proposals SET status=?,claim_token='',claimed_at=NULL WHERE id=? AND status=? AND claim_token=?`,
		models.ProposalPending, id, models.ProposalApproving, token)
	return err
}

// CompleteProposalApproval records the approved proposal's task. It reports
// false when token no longer holds the claim.
func (r *Repository) CompleteProposalApproval(ctx context.Context, id, token, taskID string, duplicate, edited bool, finalSpec *models.ProposalSpec, userID string, at time.Time) (bool, error) {
	final := ""
	if finalSpec != nil {
		data, err := json.Marshal(finalSpec)
		if err != nil {
			return false, err
		}
		final = string(data)
	}
	return r.changed(ctx, `UPDATE orchestration_task_proposals SET status=?,task_id=?,duplicate=?,edited=?,final_spec=?,decided_by=?,decided_at=?,claim_token=''
		WHERE id=? AND status=? AND claim_token=?`,
		models.ProposalApproved, taskID, duplicate, edited, final, userID, at.UTC(), id, models.ProposalApproving, token)
}

// DismissProposal records the user's dismissal of a pending proposal.
func (r *Repository) DismissProposal(ctx context.Context, id, reason, userID string, at time.Time) (bool, error) {
	return r.changed(ctx, `UPDATE orchestration_task_proposals SET status=?,dismiss_reason=?,decided_by=?,decided_at=? WHERE id=? AND status=?`,
		models.ProposalDismissed, reason, userID, at.UTC(), id, models.ProposalPending)
}

func (r *Repository) changed(ctx context.Context, query string, args ...any) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
