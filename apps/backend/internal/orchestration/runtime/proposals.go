package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const (
	proposalDecisionReason  = "proposal_decision"
	proposalDecisionsKey    = "proposal_decisions"
	proposalCommentSource   = "proposal"
	proposalOutcomeApproved = "approved"
	proposalOutcomeEdited   = "edited"
	proposalListLimit       = 50
)

// proposalDecisionUpdate tells a coordinator what the user decided about one
// of its task proposals.
type proposalDecisionUpdate struct {
	ProposalID string `json:"proposal_id"`
	Outcome    string `json:"outcome"`
	Title      string `json:"title"`
	TaskID     string `json:"task_id,omitempty"`
	Duplicate  bool   `json:"duplicate,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// ProposeTask records a create_task call as a proposal the user decides in
// chat. It returns the proposal, whether this call created it, and whether
// an undecided proposal for the same source issue was returned instead.
func (s *Service) ProposeTask(ctx context.Context, claims *runtimeauth.AgentClaims, spec models.ProposalSpec) (*models.TaskProposal, bool, bool, error) {
	sourceKey := ""
	if spec.Source != nil {
		sourceKey = spec.Source.ExternalID()
		pending, err := s.Repo.PendingProposalForSource(ctx, claims.AgentProfileID, sourceKey)
		if err != nil || pending != nil {
			return pending, false, pending != nil, err
		}
	}
	p := &models.TaskProposal{ID: uuid.NewString(), OrchestratorID: claims.AgentProfileID, WorkspaceID: claims.WorkspaceID,
		ConversationTaskID: claims.TaskID, RunID: claims.RunID, RequestHash: spec.RequestHash(), SourceKey: sourceKey,
		Spec: spec, Status: models.ProposalPending, CreatedAt: s.now()}
	comment := &models.TaskComment{TaskID: claims.TaskID, AuthorType: authorTypeAgent, AuthorID: claims.AgentProfileID,
		Source: proposalCommentSource, Body: proposalFallbackBody(spec)}
	stored, created, err := s.Repo.CreateProposal(ctx, p, comment)
	return stored, created, false, err
}

// proposalFallbackBody is the chat text of a proposal for clients that do
// not render proposal cards.
func proposalFallbackBody(spec models.ProposalSpec) string {
	var body strings.Builder
	fmt.Fprintf(&body, "**Proposed task:** %s\n\n", spec.Title)
	if description := strings.TrimSpace(spec.Description); description != "" {
		fmt.Fprintf(&body, "%s\n\n", clip(description, 1000))
	}
	body.WriteString("Approve, edit or dismiss it in the Coordinator view.")
	return body.String()
}

// ListProposals lists an orchestrator's proposals for a person reading the
// workspace. status is "pending" or "" for all.
func (s *Service) ListProposals(ctx context.Context, workspaceID, agentID, status string, limit int) ([]models.TaskProposal, error) {
	if err := s.scopeOrchestrator(ctx, workspaceID, agentID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > proposalListLimit {
		limit = proposalListLimit
	}
	rows, err := s.Repo.ListProposals(ctx, agentID, status, limit)
	if err != nil {
		return nil, err
	}
	scoped := rows[:0]
	for _, row := range rows {
		if row.WorkspaceID == workspaceID {
			scoped = append(scoped, row)
		}
	}
	return scoped, nil
}

// GetProposal returns one of an orchestrator's proposals.
func (s *Service) GetProposal(ctx context.Context, workspaceID, agentID, id string) (*models.TaskProposal, error) {
	if err := s.scopeOrchestrator(ctx, workspaceID, agentID); err != nil {
		return nil, err
	}
	p, err := s.Repo.GetProposal(ctx, agentID, id)
	if err != nil {
		return nil, err
	}
	if p.WorkspaceID != workspaceID {
		return nil, models.ErrProposalNotFound
	}
	return p, nil
}

// scopeOrchestrator requires agentID to be a registered orchestrator of the
// workspace. A mismatch reads as not found.
func (s *Service) scopeOrchestrator(ctx context.Context, workspaceID, agentID string) error {
	a, err := s.Repo.OrchestratorAssignment(ctx, agentID)
	if err != nil {
		return err
	}
	if a == nil || a.WorkspaceID != workspaceID {
		return models.ErrProposalNotFound
	}
	return nil
}

// DecideProposal applies a person's approve or dismiss. Both are idempotent:
// approving twice creates one task, and a repeated dismiss changes nothing.
// The coordinator is woken with the outcome; a failure to queue that turn
// never undoes the decision.
func (s *Service) DecideProposal(ctx context.Context, d models.ProposalDecision) (*models.TaskProposal, string, bool, error) {
	p, err := s.GetProposal(ctx, d.WorkspaceID, d.OrchestratorID, d.ProposalID)
	if err != nil {
		return nil, "", false, err
	}
	switch d.Action {
	case models.ProposalActionApprove:
		return s.approveProposal(ctx, p, d)
	case models.ProposalActionDismiss:
		p, err := s.dismissProposal(ctx, p, d)
		return p, "", false, err
	}
	return p, "", false, &models.ProposalInputError{Err: errors.New("action must be approve or dismiss")}
}

func (s *Service) approveProposal(ctx context.Context, p *models.TaskProposal, d models.ProposalDecision) (*models.TaskProposal, string, bool, error) {
	if settled, ok := settledApproval(p); ok {
		return settled, settled.TaskID, settled.Duplicate, decidedError(settled)
	}
	claimed, err := s.Repo.ClaimProposalApproval(ctx, p.ID)
	if err != nil {
		return p, "", false, err
	}
	if !claimed {
		return s.reloadDecided(ctx, p)
	}
	final, edited := p.Spec.Apply(d.Edits)
	taskID, duplicate, err := s.createProposedTask(ctx, p, &final)
	if err != nil {
		if releaseErr := s.Repo.ReleaseProposalClaim(ctx, p.ID); releaseErr != nil {
			return p, "", false, releaseErr
		}
		p.Status = models.ProposalPending
		return p, "", false, &models.ProposalInputError{Err: err}
	}
	var finalSpec *models.ProposalSpec
	if edited {
		finalSpec = &final
	}
	if _, err := s.Repo.CompleteProposalApproval(ctx, p.ID, taskID, duplicate, edited, finalSpec, d.UserID, s.now()); err != nil {
		return p, "", false, err
	}
	stored, err := s.Repo.GetProposal(ctx, p.OrchestratorID, p.ID)
	if err != nil {
		return p, "", false, err
	}
	s.wakeProposalDecision(ctx, stored)
	return stored, stored.TaskID, stored.Duplicate, nil
}

// settledApproval reports a proposal whose approval can no longer change:
// approved returns the stored result, dismissed is refused.
func settledApproval(p *models.TaskProposal) (*models.TaskProposal, bool) {
	return p, p.Status == models.ProposalApproved || p.Status == models.ProposalDismissed
}

func decidedError(p *models.TaskProposal) error {
	if p.Status == models.ProposalDismissed {
		return models.ErrProposalDecided
	}
	return nil
}

// reloadDecided answers an approval that lost its claim to another decision.
func (s *Service) reloadDecided(ctx context.Context, p *models.TaskProposal) (*models.TaskProposal, string, bool, error) {
	current, err := s.Repo.GetProposal(ctx, p.OrchestratorID, p.ID)
	if err != nil {
		return p, "", false, err
	}
	if current.Status == models.ProposalApproved {
		return current, current.TaskID, current.Duplicate, nil
	}
	return current, "", false, models.ErrProposalDecided
}

// createProposedTask validates the final spec and creates its task exactly
// as create_task would. A task that already exists for the proposal or its
// source issue is returned as a duplicate.
func (s *Service) createProposedTask(ctx context.Context, p *models.TaskProposal, final *models.ProposalSpec) (string, bool, error) {
	goal, err := validateProposalSpec(final, s.now())
	if err != nil {
		return "", false, err
	}
	if final.Source != nil {
		existing, _, err := s.Repo.TaskForSourceIssue(ctx, p.WorkspaceID, final.Source.MetadataKey(), final.Source.Key, final.Source.ExternalID())
		if err != nil || existing != "" {
			return existing, existing != "", err
		}
	}
	spec := workspaceTaskSpec(p.WorkspaceID, p.OrchestratorID, *final, goal)
	if spec.ExternalID == "" {
		spec.ExternalID = "orchestration-proposal:" + p.ID
	}
	id, err := s.Manager.CreateWorkspaceTask(ctx, spec)
	var duplicate *models.DuplicateTaskError
	if errors.As(err, &duplicate) {
		return duplicate.TaskID, true, nil
	}
	return id, false, err
}

// validateProposalSpec checks a spec a person approved. Human titles are
// never shortened.
func validateProposalSpec(spec *models.ProposalSpec, now time.Time) (*models.TaskGoal, error) {
	spec.Title = strings.TrimSpace(spec.Title)
	if spec.Title == "" {
		return nil, errors.New("title is required")
	}
	if err := taskservice.ValidateTaskTitle(spec.Title); err != nil {
		return nil, err
	}
	switch spec.ExecutionMode {
	case "", executionModeExecute, executionModeDesign:
	default:
		return nil, errors.New("execution_mode must be design or execute")
	}
	return models.NewTaskGoal(spec.AcceptanceCriteria, now)
}

// workspaceTaskSpec is the delegated task a create_task request or approved
// proposal creates.
func workspaceTaskSpec(workspaceID, chiefID string, spec models.ProposalSpec, goal *models.TaskGoal) models.WorkspaceTaskSpec {
	externalID := spec.ExternalID
	if externalID == "" && spec.Source != nil {
		externalID = spec.Source.ExternalID()
	}
	return models.WorkspaceTaskSpec{WorkspaceID: workspaceID, ChiefID: chiefID, WorkflowID: spec.WorkflowID, WorkflowStepID: spec.WorkflowStepID,
		ExecutionMode: spec.ExecutionMode, RepositoryID: spec.RepositoryID, AssigneeID: spec.AssigneeID, Title: spec.Title,
		Description: spec.Description, ExternalID: externalID, ParentID: spec.ParentID, Source: spec.Source, Goal: goal}
}

func (s *Service) dismissProposal(ctx context.Context, p *models.TaskProposal, d models.ProposalDecision) (*models.TaskProposal, error) {
	reason := strings.TrimSpace(d.Reason)
	if !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > models.ProposalDismissReasonMaxRunes {
		return p, &models.ProposalInputError{Err: fmt.Errorf("reason must be at most %d characters", models.ProposalDismissReasonMaxRunes)}
	}
	switch p.Status {
	case models.ProposalDismissed:
		return p, nil
	case models.ProposalApproved, models.ProposalApproving:
		return p, models.ErrProposalDecided
	}
	dismissed, err := s.Repo.DismissProposal(ctx, p.ID, reason, d.UserID, s.now())
	if err != nil {
		return p, err
	}
	current, err := s.Repo.GetProposal(ctx, p.OrchestratorID, p.ID)
	if err != nil {
		return p, err
	}
	if !dismissed {
		return current, decidedUnlessDismissed(current)
	}
	s.wakeProposalDecision(ctx, current)
	return current, nil
}

func decidedUnlessDismissed(p *models.TaskProposal) error {
	if p.Status == models.ProposalDismissed {
		return nil
	}
	return models.ErrProposalDecided
}

// wakeProposalDecision queues the coordinator turn that reports a decision.
// It does not change the conversation's intent revision.
func (s *Service) wakeProposalDecision(ctx context.Context, p *models.TaskProposal) {
	update := proposalDecisionUpdate{ProposalID: p.ID, Title: p.Effective().Title, TaskID: p.TaskID, Duplicate: p.Duplicate}
	switch {
	case p.Status == models.ProposalDismissed:
		update.Outcome = models.ProposalDismissed
		update.Reason, _ = clipRunes(p.DismissReason, models.ProposalDismissReasonMaxRunes)
	case p.Edited:
		update.Outcome = proposalOutcomeEdited
	default:
		update.Outcome = proposalOutcomeApproved
	}
	payload := map[string]any{proposalDecisionsKey: []proposalDecisionUpdate{update}}
	if err := s.QueueTurn(ctx, p.OrchestratorID, p.ConversationTaskID, proposalDecisionReason, "proposal-decision:"+p.ID, payload); err != nil {
		logger.Default().Warn("orchestration: proposal decision recorded but the coordinator was not woken",
			zap.String("proposal_id", p.ID), zap.String("agent_id", p.OrchestratorID), zap.Error(err))
	}
}

// payloadProposalDecisions reads a run payload's proposal decisions.
func payloadProposalDecisions(payload map[string]any) []proposalDecisionUpdate {
	raw, ok := payload[proposalDecisionsKey]
	if !ok {
		return nil
	}
	var decisions []proposalDecisionUpdate
	data, _ := json.Marshal(raw)
	_ = json.Unmarshal(data, &decisions)
	return decisions
}

// writeProposalDecisions renders the user's decisions on task proposals.
func writeProposalDecisions(text *strings.Builder, decisions []proposalDecisionUpdate) {
	if len(decisions) == 0 {
		return
	}
	text.WriteString("\nTask proposal decisions (made by the user in chat):\n")
	for _, d := range decisions {
		fmt.Fprintf(text, "- %q (proposal_id=%s): ", d.Title, d.ProposalID)
		switch d.Outcome {
		case models.ProposalDismissed:
			text.WriteString("dismissed.")
			if d.Reason != "" {
				fmt.Fprintf(text, " Reason: %q", d.Reason)
			}
		case proposalOutcomeEdited:
			fmt.Fprintf(text, "approved with the user's edits; created task_id=%s", d.TaskID)
		default:
			fmt.Fprintf(text, "approved; created task_id=%s", d.TaskID)
		}
		if d.Duplicate {
			text.WriteString(" [already existed]")
		}
		text.WriteString("\n")
	}
	text.WriteString("Do not propose a dismissed task again unless the user asks.\n")
}
