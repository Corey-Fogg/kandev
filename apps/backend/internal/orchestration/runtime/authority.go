package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const workspaceCoordinatorAudience = "workspace_coordinator"
const statusClaimed = "claimed"

// ErrOrchestrationDisabled settles work while features.orchestration is off.
var ErrOrchestrationDisabled = errors.New("orchestration feature disabled")

// ErrOrchestratorPaused refuses new work for a paused coordinator.
var ErrOrchestratorPaused = errors.New("orchestrator_paused")

// CheckConversationExecution refuses work on a paused coordinator's
// conversation. Tasks that are not coordinator conversations pass.
func (s *Service) CheckConversationExecution(ctx context.Context, taskID string) error {
	persona, _, err := s.Repo.ConversationOwner(ctx, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	a, err := s.Personas.GetAgentInstance(ctx, persona)
	if err != nil {
		return err
	}
	if paused(a) {
		return ErrOrchestratorPaused
	}
	return nil
}

// CheckCoordinatorSession rejects a native launch, resume or steer of a
// coordinator conversation unless the session is pinned to the broker surface
// and belongs to the conversation's claimed run at the current intent.
func (s *Service) CheckCoordinatorSession(ctx context.Context, taskID string, session *taskmodels.TaskSession) error {
	if !s.Enabled {
		return ErrOrchestrationDisabled
	}
	if err := s.CheckConversationExecution(ctx, taskID); err != nil {
		return err
	}
	if session == nil || session.TaskID != taskID || !mcpprofile.SessionUsesBroker(session.Metadata) {
		return models.ErrConflict
	}
	run, err := s.Runs.LatestRunForSession(ctx, session.ID)
	if err != nil || run.Status != statusClaimed {
		return models.ErrConflict
	}
	var snapshot struct {
		Revision int64 `json:"intent_revision"`
	}
	if json.Unmarshal([]byte(run.Payload), &snapshot) != nil {
		return models.ErrConflict
	}
	revision, err := s.Repo.IntentRevision(ctx, taskID)
	if err != nil {
		return err
	}
	if revision != snapshot.Revision {
		return models.ErrConflict
	}
	return nil
}
