package linear

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
)

// IssueCommenter is implemented by clients that can comment on an issue.
type IssueCommenter interface {
	AddComment(ctx context.Context, issueID, body string) error
}

// ErrCommentUnsupported means the configured client cannot post comments.
var ErrCommentUnsupported = errors.New("linear: client cannot post comments")

const commentCreateMutation = `
mutation CommentCreate($issueId: String!, $body: String!) {
	commentCreate(input: { issueId: $issueId, body: $body }) {
		success
	}
}`

// AddComment posts a Markdown comment on an issue addressed by its UUID.
func (c *GraphQLClient) AddComment(ctx context.Context, issueID, body string) error {
	var data struct {
		CommentCreate struct {
			Success bool `json:"success"`
		} `json:"commentCreate"`
	}
	if err := c.do(ctx, commentCreateMutation, map[string]interface{}{"issueId": issueID, "body": body}, &data); err != nil {
		return err
	}
	if !data.CommentCreate.Success {
		return &APIError{StatusCode: http.StatusInternalServerError, Message: "commentCreate returned success=false"}
	}
	return nil
}

// AddCommentForWorkspace comments on an issue, addressed by identifier, with
// one workspace's client.
func (s *Service) AddCommentForWorkspace(ctx context.Context, workspaceID, identifier, body string) error {
	client, err := s.workspaceClient(ctx, workspaceID)
	if err != nil {
		return err
	}
	commenter, ok := client.(IssueCommenter)
	if !ok {
		return ErrCommentUnsupported
	}
	issue, err := client.GetIssue(ctx, identifier)
	if err != nil {
		return err
	}
	return commenter.AddComment(ctx, issue.ID, body)
}

// StateMatch is the workflow state chosen for an issue. Current means the
// issue already sits in a qualifying state; Available lists the team's
// states when none qualifies.
type StateMatch struct {
	State     *LinearWorkflowState
	Current   bool
	Available []string
}

// MoveToStateTypeForWorkspace moves an issue to a workflow state of stateType
// (backlog, unstarted, started, completed or canceled). A state whose name
// contains prefer ranks first; with strict, only such a state qualifies.
// Within a rank the lowest position wins.
func (s *Service) MoveToStateTypeForWorkspace(ctx context.Context, workspaceID, identifier, stateType, prefer string, strict bool) (StateMatch, error) {
	client, err := s.workspaceClient(ctx, workspaceID)
	if err != nil {
		return StateMatch{}, err
	}
	issue, err := client.GetIssue(ctx, identifier)
	if err != nil {
		return StateMatch{}, err
	}
	if issue.StateType == stateType && (!strict || strings.Contains(strings.ToLower(issue.StateName), prefer)) {
		return StateMatch{Current: true}, nil
	}
	states := issue.States
	if len(states) == 0 {
		if states, err = client.ListStates(ctx, issue.TeamKey); err != nil {
			return StateMatch{}, err
		}
	}
	match := matchState(states, stateType, prefer, strict)
	if match.State == nil {
		return match, nil
	}
	return match, client.SetIssueState(ctx, issue.ID, match.State.ID)
}

func matchState(states []LinearWorkflowState, stateType, prefer string, strict bool) StateMatch {
	ordered := append([]LinearWorkflowState(nil), states...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Position < ordered[j].Position })
	match := StateMatch{}
	preferred := false
	for i := range ordered {
		state := &ordered[i]
		match.Available = append(match.Available, state.Name+" ("+state.Type+")")
		if state.Type != stateType {
			continue
		}
		mentions := prefer != "" && strings.Contains(strings.ToLower(state.Name), prefer)
		if (match.State == nil && !strict) || (mentions && !preferred) {
			match.State = state
			preferred = mentions
		}
	}
	return match
}

func (s *Service) workspaceClient(ctx context.Context, workspaceID string) (Client, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return nil, err
	}
	return s.clientFor(ctx, workspaceID)
}
