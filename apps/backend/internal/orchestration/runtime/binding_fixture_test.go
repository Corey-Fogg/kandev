package runtime

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

// bindTestAssistant stores a binding row for the binding-keyed stores under
// test. Coordinator conversations never read it.
func bindTestAssistant(t *testing.T, s *Service, db *sqlx.DB, owner, orchestrator, task string) *models.AssistantBinding {
	t.Helper()
	_, err := db.Exec(`INSERT INTO orchestration_assistant_bindings
		(id,owner_user_id,orchestrator_id,workspace_id,conversation_id,version,created_at,updated_at)
		VALUES(?,?,?,'ws',?,1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, "binding-"+owner, owner, orchestrator, task)
	require.NoError(t, err)
	b, err := s.Repo.AssistantBinding(context.Background(), owner)
	require.NoError(t, err)
	return b
}
