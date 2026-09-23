package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// History may already contain foreign exports. Changing the receiver therefore
// requires every historical field to be confirmed for that receiver, even when
// the original grant has since been revoked or the local packet cache is empty.
func (s *Service) validateWorkspaceHistory(ctx context.Context, b *models.AssistantBinding, authority models.AssistantAuthority, record bool) error {
	rows, err := s.workspaceHistory(ctx, b)
	if err != nil || len(rows) == 0 {
		return err
	}
	revision, err := s.contextProfileRevision(ctx, b.WorkspaceID, authority.ProfileID)
	if err != nil {
		return err
	}
	already := workspaceDeliveredFields(rows, authority, revision)
	checked := map[string]*models.WorkspaceGrant{}
	for _, row := range rows {
		key := row.WorkspaceID + ":" + row.Kind
		if already[key] || checked[key] != nil {
			continue
		}
		g, err := s.reconfirmedHistoryGrant(ctx, b, row)
		if err != nil {
			return fmt.Errorf("history requires workspace reconfirmation")
		}
		checked[key] = g
	}
	if record {
		for _, row := range rows {
			key := row.WorkspaceID + ":" + row.Kind
			if g := checked[key]; g != nil {
				if err = s.Repo.RecordWorkspaceExport(ctx, b, g, row.Kind); err != nil {
					return err
				}
				delete(checked, key)
			}
		}
	}
	return nil
}

func workspaceDeliveredFields(rows []models.WorkspaceExport, authority models.AssistantAuthority, revision string) map[string]bool {
	already := map[string]bool{}
	for _, row := range rows {
		if row.ReceiverProfileID == authority.ProfileID && row.ReceiverProfileRevision == revision && row.AuthorityRevision == authority.Revision {
			already[row.WorkspaceID+":"+row.Kind] = true
		}
	}
	return already
}

func (s *Service) reconfirmedHistoryGrant(ctx context.Context, b *models.AssistantBinding, row models.WorkspaceExport) (*models.WorkspaceGrant, error) {
	g, err := s.Repo.WorkspaceGrant(ctx, b.ID, row.WorkspaceID)
	if err != nil {
		return nil, err
	}
	kind := row.Kind
	if kind == "workspace_link" {
		kind = ""
	}
	return s.currentWorkspaceGrant(ctx, b, row.WorkspaceID, g.Revision, workspaceObserve, kind)
}

func (s *Service) workspaceHistory(ctx context.Context, b *models.AssistantBinding) ([]models.WorkspaceExport, error) {
	rows := []models.WorkspaceExport{}
	after := ""
	for len(rows) < 10000 {
		page, err := s.Repo.WorkspaceExports(ctx, b.ID, b.ConversationID, after, 100)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page...)
		if len(page) < 100 {
			return rows, nil
		}
		after = page[len(page)-1].ID
	}
	return nil, fmt.Errorf("workspace history exceeds the validation budget")
}

// AssistantAuthorityReader fingerprints the configuration that workspace
// grants and maintenance grants were issued against.
type AssistantAuthorityReader interface {
	ResolveAssistantAuthority(context.Context, models.AssistantBinding, string, string) (models.AssistantAuthority, error)
}

func (s *Service) assistantAuthority(ctx context.Context, taskID string) (*models.AssistantAuthority, error) {
	binding, err := s.Repo.AssistantForConversation(ctx, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !s.Enabled {
		return nil, ErrOrchestrationDisabled
	}
	persona, err := s.Personas.GetAgentInstance(ctx, binding.OrchestratorID)
	if err != nil {
		return nil, err
	}
	if paused(persona) {
		return nil, ErrOrchestratorPaused
	}
	profile, executor, err := s.executionSelection(ctx, persona)
	if err != nil {
		return nil, err
	}
	if s.Authority == nil {
		return nil, errors.New("assistant_authority_unavailable")
	}
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: binding.OwnerUserID, Role: authn.RoleMember})
	row, err := s.Authority.ResolveAssistantAuthority(ctx, *binding, profile, executor)
	if err != nil {
		return nil, err
	}
	row.ProfileID, row.ExecutorID = profile, executor
	row.BindingID, row.BindingVersion = binding.ID, binding.Version
	row.IntentRevision, err = s.Repo.IntentRevision(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err = s.validateWorkspaceHistory(ctx, binding, row, false); err != nil {
		return nil, err
	}
	return &row, nil
}
