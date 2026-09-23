package models

import "errors"

// ErrConflict reports a lost compare-and-swap or a reused idempotency key.
var ErrConflict = errors.New("revision or idempotency conflict")

type Intake struct {
	CommentID       string `db:"comment_id"`
	TaskID          string `db:"task_id"`
	AgentID         string `db:"agent_id"`
	OwnerUserID     string `db:"owner_user_id"`
	ClientMessageID string `db:"client_message_id"`
	PayloadHash     string `db:"payload_hash"`
	Sequence        int64  `db:"sequence"`
	Status          string `db:"status"`
	RunID           string `db:"run_id"`
}
