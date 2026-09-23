package sqlite

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/orchestration/models"
)

const commentSelect = `SELECT c.id,c.task_id,c.author_type,c.author_id,c.body,c.source,c.reply_channel_id,c.created_at,
	COALESCE(i.client_message_id,'') AS client_message_id,COALESCE(i.sequence,0) AS sequence,
	COALESCE(i.sequence,0) AS intent_revision,COALESCE(i.status,'') AS receipt_status,COALESCE(i.run_id,'') AS run_id
	FROM task_comments c LEFT JOIN orchestration_intake i ON i.comment_id=c.id`

func (r *Repository) GetCommentByID(ctx context.Context, taskID, id string) (*models.TaskComment, error) {
	var row models.TaskComment
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind(commentSelect+` WHERE c.task_id=? AND c.id=?`), taskID, id)
	return &row, err
}

func (r *Repository) ListComments(ctx context.Context, taskID string, limit int) ([]*models.TaskComment, error) {
	return r.CommentsBefore(ctx, taskID, "", limit)
}

func (r *Repository) CommentsBefore(ctx context.Context, taskID, before string, limit int) ([]*models.TaskComment, error) {
	rows := []*models.TaskComment{}
	query, args := commentSelect+` WHERE c.task_id=?`, []any{taskID}
	if before != "" {
		if _, err := r.GetCommentByID(ctx, taskID, before); err != nil {
			return nil, err
		}
		query += ` AND (c.created_at,c.id)<(SELECT created_at,id FROM task_comments WHERE task_id=? AND id=?)`
		args = append(args, taskID, before)
	}
	query += ` ORDER BY c.created_at DESC,c.id DESC LIMIT ?`
	args = append(args, limit)
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(query), args...)
	return rows, err
}

// UnansweredMessages returns, oldest first, the user messages of a
// conversation accepted after its last completed user-message turn and
// before revision. Superseded turns never ran, so their messages are here.
// runID is the turn being rendered; it never counts as completed.
func (r *Repository) UnansweredMessages(ctx context.Context, agentID, taskID, runID string, revision int64, limit int) ([]*models.TaskComment, error) {
	commentID := dialect.JSONExtract(r.ro.DriverName(), "r.payload", "comment_id")
	var floor int64
	err := r.ro.GetContext(ctx, &floor, r.ro.Rebind(`SELECT COALESCE(MAX(i.sequence),0) FROM runs r
		JOIN orchestration_intake i ON i.comment_id=`+commentID+`
		WHERE r.agent_profile_id=? AND r.reason='task_comment' AND r.status='finished'
		  AND COALESCE(r.outcome,'')<>'superseded' AND r.id<>? AND i.task_id=?`), agentID, runID, taskID)
	if err != nil {
		return nil, err
	}
	rows := []*models.TaskComment{}
	err = r.ro.SelectContext(ctx, &rows, r.ro.Rebind(commentSelect+`
		WHERE c.task_id=? AND c.author_type='user' AND i.sequence>? AND i.sequence<?
		ORDER BY i.sequence DESC LIMIT ?`), taskID, floor, revision, limit)
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, err
}

func (r *Repository) PutComment(ctx context.Context, c *models.TaskComment) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind("INSERT INTO task_comments(id,task_id,author_type,author_id,body,source,created_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING"), c.ID, c.TaskID, c.AuthorType, c.AuthorID, c.Body, c.Source, c.CreatedAt)
	return err
}
func (r *Repository) ConversationOwner(ctx context.Context, taskID string) (string, string, error) {
	var row struct {
		AgentID     string `db:"agent_id"`
		WorkspaceID string `db:"workspace_id"`
	}
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind("SELECT o.agent_id,o.workspace_id FROM workspace_orchestrators o JOIN orchestration_conversations c ON c.agent_profile_id=o.agent_id JOIN tasks t ON t.id=c.task_id WHERE c.task_id=? AND c.platform='web' AND c.workspace_id=o.workspace_id AND t.workspace_id=o.workspace_id"), taskID)
	return row.AgentID, row.WorkspaceID, err
}
