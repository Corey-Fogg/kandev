package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/orchestration/models"
)

// bindingColumns lists the binding fields read into models.AssistantBinding.
// Reads name their columns so retained legacy columns never break the scan.
const bindingColumns = `b.id,b.owner_user_id,b.orchestrator_id,b.workspace_id,b.conversation_id,b.version,b.created_at,b.updated_at`

func (r *Repository) AssistantBinding(ctx context.Context, owner string) (*models.AssistantBinding, error) {
	var row models.AssistantBinding
	err := r.db.GetContext(ctx, &row, r.db.Rebind(`SELECT `+bindingColumns+` FROM orchestration_assistant_bindings b
		JOIN workspace_orchestrators o ON o.agent_id=b.orchestrator_id AND o.workspace_id=b.workspace_id
		JOIN tasks t ON t.id=b.conversation_id AND t.workspace_id=b.workspace_id
		WHERE b.owner_user_id=?`), owner)
	return &row, err
}
