package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

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
	return nil
}

const proposalColumns = `id,agent_id,workspace_id,conversation_task_id,run_id,request_hash,source_key,spec,final_spec,edited,status,task_id,duplicate,dismiss_reason,decided_by,created_at,decided_at`

// CreateProposal stores a proposal and its conversation comment together.
// The comment's id is the proposal id. A replay of the same request in the
// same run returns the stored proposal with created=false.
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
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_task_proposals
		(id,agent_id,workspace_id,conversation_task_id,run_id,request_hash,source_key,spec,status,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?) ON CONFLICT(agent_id,run_id,request_hash) DO NOTHING`),
		p.ID, p.OrchestratorID, p.WorkspaceID, p.ConversationTaskID, p.RunID, p.RequestHash, p.SourceKey, p.SpecJSON, models.ProposalPending, p.CreatedAt)
	if err != nil {
		return nil, false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if n == 0 {
		// Release the write transaction before reading the stored replay.
		_ = tx.Rollback()
		existing, err := r.proposalWhere(ctx, `agent_id=? AND run_id=? AND request_hash=?`, p.OrchestratorID, p.RunID, p.RequestHash)
		return existing, false, err
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

// ClaimProposalApproval marks an undecided proposal as being approved. An
// interrupted approval can be claimed again.
func (r *Repository) ClaimProposalApproval(ctx context.Context, id string) (bool, error) {
	return r.changed(ctx, `UPDATE orchestration_task_proposals SET status=? WHERE id=? AND status IN (?,?)`,
		models.ProposalApproving, id, models.ProposalPending, models.ProposalApproving)
}

// ReleaseProposalClaim returns a failed approval to pending.
func (r *Repository) ReleaseProposalClaim(ctx context.Context, id string) error {
	_, err := r.changed(ctx, `UPDATE orchestration_task_proposals SET status=? WHERE id=? AND status=?`,
		models.ProposalPending, id, models.ProposalApproving)
	return err
}

// CompleteProposalApproval records the approved proposal's task.
func (r *Repository) CompleteProposalApproval(ctx context.Context, id, taskID string, duplicate, edited bool, finalSpec *models.ProposalSpec, userID string, at time.Time) (bool, error) {
	final := ""
	if finalSpec != nil {
		data, err := json.Marshal(finalSpec)
		if err != nil {
			return false, err
		}
		final = string(data)
	}
	return r.changed(ctx, `UPDATE orchestration_task_proposals SET status=?,task_id=?,duplicate=?,edited=?,final_spec=?,decided_by=?,decided_at=? WHERE id=? AND status=?`,
		models.ProposalApproved, taskID, duplicate, edited, final, userID, at.UTC(), id, models.ProposalApproving)
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
