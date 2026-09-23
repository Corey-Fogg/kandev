package models

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Task proposal statuses. approving means a create is in flight or was
// interrupted; approve may be retried.
const (
	ProposalPending   = "pending"
	ProposalApproving = "approving"
	ProposalApproved  = "approved"
	ProposalDismissed = "dismissed"
)

// Proposal decision actions.
const (
	ProposalActionApprove = "approve"
	ProposalActionDismiss = "dismiss"
)

// ProposalDismissReasonMaxRunes bounds a dismiss reason.
const ProposalDismissReasonMaxRunes = 500

// ProposalSpec is the task a coordinator proposes, exactly as create_task
// would have created it.
type ProposalSpec struct {
	Title              string       `json:"title"`
	Description        string       `json:"description,omitempty"`
	WorkflowID         string       `json:"workflow_id,omitempty"`
	WorkflowStepID     string       `json:"workflow_step_id,omitempty"`
	RepositoryID       string       `json:"repository_id,omitempty"`
	ParentID           string       `json:"parent_id,omitempty"`
	AssigneeID         string       `json:"assignee,omitempty"`
	ExecutionMode      string       `json:"execution_mode,omitempty"`
	ExternalID         string       `json:"external_id,omitempty"`
	Source             *SourceIssue `json:"source,omitempty"`
	AcceptanceCriteria []string     `json:"acceptance_criteria,omitempty"`
}

// ProposalEdits are the user's changes to a proposal. A nil field keeps the
// proposed value.
type ProposalEdits struct {
	Title              *string   `json:"title"`
	Description        *string   `json:"description"`
	WorkflowID         *string   `json:"workflow_id"`
	WorkflowStepID     *string   `json:"workflow_step_id"`
	RepositoryID       *string   `json:"repository_id"`
	AssigneeID         *string   `json:"assignee"`
	ExecutionMode      *string   `json:"execution_mode"`
	AcceptanceCriteria *[]string `json:"acceptance_criteria"`
}

// TaskProposal is a durable create_task proposal awaiting or after the
// user's decision. Spec and FinalSpec are stored as JSON text.
type TaskProposal struct {
	ID                 string        `json:"id" db:"id"`
	OrchestratorID     string        `json:"orchestrator_id" db:"agent_id"`
	WorkspaceID        string        `json:"workspace_id" db:"workspace_id"`
	ConversationTaskID string        `json:"conversation_task_id" db:"conversation_task_id"`
	RunID              string        `json:"-" db:"run_id"`
	RequestHash        string        `json:"-" db:"request_hash"`
	SourceKey          string        `json:"-" db:"source_key"`
	Status             string        `json:"status" db:"status"`
	Spec               ProposalSpec  `json:"spec" db:"-"`
	FinalSpec          *ProposalSpec `json:"final_spec" db:"-"`
	SpecJSON           string        `json:"-" db:"spec"`
	FinalSpecJSON      string        `json:"-" db:"final_spec"`
	Edited             bool          `json:"edited" db:"edited"`
	TaskID             string        `json:"task_id" db:"task_id"`
	Duplicate          bool          `json:"duplicate" db:"duplicate"`
	DismissReason      string        `json:"dismiss_reason" db:"dismiss_reason"`
	DecidedBy          string        `json:"decided_by" db:"decided_by"`
	CreatedAt          time.Time     `json:"created_at" db:"created_at"`
	DecidedAt          *time.Time    `json:"decided_at" db:"decided_at"`
}

// DecodeSpecs fills Spec and FinalSpec from their stored JSON.
func (p *TaskProposal) DecodeSpecs() error {
	if err := json.Unmarshal([]byte(p.SpecJSON), &p.Spec); err != nil {
		return err
	}
	p.FinalSpec = nil
	if p.FinalSpecJSON == "" {
		return nil
	}
	var final ProposalSpec
	if err := json.Unmarshal([]byte(p.FinalSpecJSON), &final); err != nil {
		return err
	}
	p.FinalSpec = &final
	return nil
}

// Effective is the spec the task was, or will be, created from.
func (p *TaskProposal) Effective() ProposalSpec {
	if p.FinalSpec != nil {
		return *p.FinalSpec
	}
	return p.Spec
}

// ProposalDecision is a user's approve or dismiss of one proposal.
type ProposalDecision struct {
	WorkspaceID, OrchestratorID, ProposalID, UserID string
	Action                                          string
	Edits                                           *ProposalEdits
	Reason                                          string
}

var (
	ErrProposalNotFound = errors.New("proposal_not_found")
	ErrProposalDecided  = errors.New("proposal_already_decided")
	// ErrProposalApproving means another approval of the proposal is still
	// creating its task.
	ErrProposalApproving = errors.New("proposal_approval_in_progress")
)

// ProposalInputError means an approval was refused: the final spec is
// invalid or the task could not be created. The proposal stays pending.
type ProposalInputError struct{ Err error }

func (e *ProposalInputError) Error() string { return e.Err.Error() }
func (e *ProposalInputError) Unwrap() error { return e.Err }

// Apply overlays edits on the spec and reports whether anything changed.
func (s ProposalSpec) Apply(e *ProposalEdits) (ProposalSpec, bool) {
	if e == nil {
		return s, false
	}
	changed := false
	for _, field := range []struct {
		edit   *string
		target *string
	}{
		{e.Title, &s.Title}, {e.Description, &s.Description}, {e.WorkflowID, &s.WorkflowID},
		{e.WorkflowStepID, &s.WorkflowStepID}, {e.RepositoryID, &s.RepositoryID},
		{e.AssigneeID, &s.AssigneeID}, {e.ExecutionMode, &s.ExecutionMode},
	} {
		if field.edit != nil && *field.edit != *field.target {
			*field.target, changed = *field.edit, true
		}
	}
	if e.AcceptanceCriteria != nil && !slices.Equal(*e.AcceptanceCriteria, s.AcceptanceCriteria) {
		s.AcceptanceCriteria, changed = slices.Clone(*e.AcceptanceCriteria), true
	}
	return s, changed
}

// RequestHash identifies the proposed spec for replay detection.
func (s ProposalSpec) RequestHash() string {
	data, _ := json.Marshal(s)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
