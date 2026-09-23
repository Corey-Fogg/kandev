package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
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

// migrateSingleOrchestratorIndex enforces one orchestrator per workspace once
// no workspace has more. Legacy duplicates are kept working; every replay
// tries again, so the index appears after the extras are deleted.
func (r *Repository) migrateSingleOrchestratorIndex() error {
	var duplicated int
	if err := r.db.Get(&duplicated, `SELECT COUNT(*) FROM (SELECT workspace_id FROM workspace_orchestrators GROUP BY workspace_id HAVING COUNT(*)>1) d`); err != nil {
		return err
	}
	if duplicated > 0 {
		logger.Default().Info("orchestration: skipping the one-orchestrator-per-workspace index while legacy workspaces have several",
			zap.Int("workspaces", duplicated))
		return nil
	}
	_, err := r.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_workspace_orchestrators_one_per_workspace ON workspace_orchestrators(workspace_id)`)
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

// RegisterOrchestrator creates or imports a workspace's orchestrator. A
// workspace has at most one: registering another returns
// models.ErrOrchestratorExists.
func (r *Repository) RegisterOrchestrator(ctx context.Context, agentID, workspaceID, roleID string) error {
	defer r.invalidateRegistered()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO workspace_orchestrators (agent_id,workspace_id,role_id)
		SELECT id,workspace_id,? FROM agent_profiles
		WHERE id=? AND workspace_id=? AND deleted_at IS NULL AND role='assistant'
		  AND NOT EXISTS (SELECT 1 FROM workspace_orchestrators o WHERE o.workspace_id=? AND o.agent_id<>?)
		ON CONFLICT(agent_id) DO UPDATE SET role_id=excluded.role_id`), roleID, agentID, workspaceID, workspaceID, agentID)
	if isUniqueViolation(err) {
		return models.ErrOrchestratorExists
	}
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 1 {
		return err
	}
	var others int
	if err := r.db.GetContext(ctx, &others, r.db.Rebind(`SELECT COUNT(*) FROM workspace_orchestrators WHERE workspace_id=? AND agent_id<>?`), workspaceID, agentID); err != nil {
		return err
	}
	if others > 0 {
		return models.ErrOrchestratorExists
	}
	return fmt.Errorf("orchestrator must belong to this workspace")
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed: workspace_orchestrators.workspace_id")
}

// UpdateOrchestratorRole changes an existing orchestrator's role and
// refreshes its cached profile name. Legacy duplicate assignments stay
// editable.
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
