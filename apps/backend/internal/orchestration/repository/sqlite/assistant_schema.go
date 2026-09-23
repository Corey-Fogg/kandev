package sqlite

// migrateIntake creates the conversation intake outbox and its intent
// revision counter.
func (r *Repository) migrateIntake() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS orchestration_conversation_intents (
			task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE, revision INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS orchestration_intake (
			comment_id TEXT PRIMARY KEY REFERENCES task_comments(id) ON DELETE CASCADE,
			task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			agent_id TEXT NOT NULL REFERENCES workspace_orchestrators(agent_id) ON DELETE CASCADE,
			owner_user_id TEXT NOT NULL, client_message_id TEXT NOT NULL, payload_hash TEXT NOT NULL,
			sequence INTEGER NOT NULL, status TEXT NOT NULL DEFAULT 'accepted', run_id TEXT NOT NULL DEFAULT '',
			UNIQUE(task_id,owner_user_id,client_message_id), UNIQUE(task_id,sequence))`,
		`CREATE INDEX IF NOT EXISTS idx_orchestration_intake_pending ON orchestration_intake(status,task_id,sequence)`,
	} {
		if _, err := r.db.Exec(renderSchema(r.db.DriverName(), q)); err != nil {
			return err
		}
	}
	return nil
}

// migrateAssistantStorage creates the binding table that owner-scoped memory
// reads. Coordinator conversations never read a binding, so a retained binding
// row has no effect on them.
func (r *Repository) migrateAssistantStorage() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS orchestration_assistant_bindings (
			id TEXT PRIMARY KEY, owner_user_id TEXT NOT NULL UNIQUE,
			orchestrator_id TEXT NOT NULL UNIQUE REFERENCES workspace_orchestrators(agent_id) ON DELETE CASCADE,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			conversation_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			version INTEGER NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
	} {
		if _, err := r.db.Exec(renderSchema(r.db.DriverName(), q)); err != nil {
			return err
		}
	}
	return nil
}
