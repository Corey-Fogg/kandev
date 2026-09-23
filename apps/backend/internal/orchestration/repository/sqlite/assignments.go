package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestration/models"
)

const errOrchestratorNotRegistered = "orchestrator is not registered"

// migrateAssignmentSettings appends the instance name and behavior settings
// to workspace orchestrator assignments.
func (r *Repository) migrateAssignmentSettings() error {
	for _, column := range []struct{ name, definition string }{
		{"display_name", `TEXT NOT NULL DEFAULT ''`},
		{"ask_before_create", `{{boolean}} NOT NULL DEFAULT FALSE`},
		{"auto_comment_source", `{{boolean}} NOT NULL DEFAULT TRUE`},
		{"auto_move_source_done", `{{boolean}} NOT NULL DEFAULT FALSE`},
	} {
		exists, err := db.ColumnExists(r.db, "workspace_orchestrators", column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		statement := renderSchema(r.db.DriverName(), `ALTER TABLE workspace_orchestrators ADD COLUMN `+column.name+` `+column.definition)
		if _, err := r.db.Exec(statement); err != nil && !db.IsDuplicateColumnError(err) {
			return err
		}
	}
	return nil
}

// migrateDropSingleOrchestratorIndex removes the one-orchestrator-per-workspace
// index an earlier build created. A workspace may have several orchestrators.
func (r *Repository) migrateDropSingleOrchestratorIndex() error {
	_, err := r.db.Exec(`DROP INDEX IF EXISTS ux_workspace_orchestrators_one_per_workspace`)
	return err
}

const assignmentSelect = `SELECT o.agent_id,o.workspace_id,o.role_id,o.display_name,o.ask_before_create,o.auto_comment_source,o.auto_move_source_done
	FROM workspace_orchestrators o JOIN agent_profiles a ON a.id=o.agent_id WHERE o.agent_id=? AND a.deleted_at IS NULL`

// OrchestratorAssignment returns an orchestrator's registration, or nil when
// the profile is not a registered orchestrator.
func (r *Repository) OrchestratorAssignment(ctx context.Context, agentID string) (*models.OrchestratorAssignment, error) {
	var row models.OrchestratorAssignment
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind(assignmentSelect), agentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// RegisterOrchestrator creates or imports one of a workspace's orchestrators.
// A workspace may have several.
func (r *Repository) RegisterOrchestrator(ctx context.Context, agentID, workspaceID, roleID string) error {
	defer r.invalidateRegistered()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO workspace_orchestrators (agent_id,workspace_id,role_id)
		SELECT id,workspace_id,? FROM agent_profiles
		WHERE id=? AND workspace_id=? AND deleted_at IS NULL AND role='assistant'
		ON CONFLICT(agent_id) DO UPDATE SET role_id=excluded.role_id`), roleID, agentID, workspaceID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return fmt.Errorf("orchestrator must belong to this workspace")
	}
	return err
}

// UpdateOrchestratorRole changes an existing orchestrator's role and
// refreshes its cached profile name.
func (r *Repository) UpdateOrchestratorRole(ctx context.Context, agentID, roleID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workspace_orchestrators SET role_id=? WHERE agent_id=?`), roleID, agentID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(err, errors.New(errOrchestratorNotRegistered))
	}
	if _, err := syncProfileName(ctx, tx, agentID); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveOrchestratorSettings stores an orchestrator's behavior settings.
func (r *Repository) SaveOrchestratorSettings(ctx context.Context, agentID string, s models.OrchestratorSettings) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE workspace_orchestrators SET ask_before_create=?,auto_comment_source=?,auto_move_source_done=? WHERE agent_id=?`),
		s.AskBeforeCreate, s.AutoCommentSource, s.AutoMoveSourceDone, agentID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(err, errors.New(errOrchestratorNotRegistered))
	}
	return nil
}

// SetOrchestratorDisplayName renames an orchestrator instance. Its profile
// name and conversation title follow the effective name, which it returns.
func (r *Repository) SetOrchestratorDisplayName(ctx context.Context, agentID, displayName string) (string, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE workspace_orchestrators SET display_name=? WHERE agent_id=?`), displayName, agentID)
	if err != nil {
		return "", err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return "", errors.Join(err, errors.New(errOrchestratorNotRegistered))
	}
	effective, err := syncProfileName(ctx, tx, agentID)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE tasks SET title=? WHERE id=(SELECT task_id FROM orchestration_conversations WHERE agent_profile_id=? AND platform='web')`),
		"Conversation with "+effective, agentID); err != nil {
		return "", err
	}
	return effective, tx.Commit()
}

// syncProfileName sets the core profile name to the orchestrator's effective
// name and returns it.
func syncProfileName(ctx context.Context, tx execGetter, agentID string) (string, error) {
	var row struct {
		DisplayName string `db:"display_name"`
		RoleName    string `db:"role_name"`
	}
	if err := tx.GetContext(ctx, &row, tx.Rebind(`SELECT o.display_name,r.name AS role_name FROM workspace_orchestrators o JOIN orchestration_roles r ON r.id=o.role_id WHERE o.agent_id=?`), agentID); err != nil {
		return "", err
	}
	effective := models.EffectiveName(row.DisplayName, row.RoleName)
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_profiles SET name=? WHERE id=?`), effective, agentID)
	return effective, err
}

type execGetter interface {
	GetContext(ctx context.Context, dest any, query string, args ...any) error
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Rebind(query string) string
}

// ConversationTaskID returns an orchestrator's web conversation task, or ""
// when it has none yet. It never creates one.
func (r *Repository) ConversationTaskID(ctx context.Context, agentID string) (string, error) {
	var id string
	err := r.ro.GetContext(ctx, &id, r.ro.Rebind(`SELECT task_id FROM orchestration_conversations WHERE agent_profile_id=? AND platform='web'`), agentID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
