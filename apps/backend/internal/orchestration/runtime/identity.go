package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/orchestration/models"
)

// writeIdentity renders the orchestrator's instance name and the settings
// that change how its tools behave.
func (s *Service) writeIdentity(ctx context.Context, text *strings.Builder, a *models.AgentInstance) error {
	assignment, err := s.Repo.OrchestratorAssignment(ctx, a.ID)
	if err != nil {
		return err
	}
	settings := models.DefaultOrchestratorSettings()
	if assignment != nil {
		settings = assignment.OrchestratorSettings
	}
	fmt.Fprintf(text, "Your name in this workspace: %s\n", a.Name)
	fmt.Fprintf(text, "Settings: ask before creating tasks=%s; automatic source issue comment=%s; automatic move of source issue to done on completion=%s\n",
		onOff(settings.AskBeforeCreate), onOff(settings.AutoCommentSource), onOff(settings.AutoMoveSourceDone))
	return nil
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

// assignment returns the caller's registration, or an error when the
// profile is not a registered orchestrator.
func (s *Service) assignment(ctx context.Context, agentID string) (*models.OrchestratorAssignment, error) {
	a, err := s.Repo.OrchestratorAssignment(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("coordinator not registered")
	}
	return a, nil
}
