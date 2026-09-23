package jira

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// IssueCommenter is implemented by clients that can comment on a ticket.
type IssueCommenter interface {
	AddComment(ctx context.Context, ticketKey, body string) error
}

// ErrCommentUnsupported means the configured client cannot post comments.
var ErrCommentUnsupported = errors.New("jira: client cannot post comments")

// AddComment posts a plain-text comment. Cloud (API v3) takes an Atlassian
// Document Format body with one paragraph per line; Server (API v2) takes
// the text as is.
func (c *CloudClient) AddComment(ctx context.Context, ticketKey, body string) error {
	path := c.apiBase + "/issue/" + url.PathEscape(ticketKey) + "/comment"
	if c.instanceType == InstanceTypeServer {
		return c.do(ctx, http.MethodPost, path, map[string]any{"body": body}, nil)
	}
	return c.do(ctx, http.MethodPost, path, map[string]any{"body": adfDocument(body)}, nil)
}

// AddComment posts a Markdown comment through the Atlassian MCP server.
func (c *MCPClient) AddComment(ctx context.Context, ticketKey, body string) error {
	const toolName, toolArguments, cloudID, issueKey = "name", "arguments", "cloudId", "issueIdOrKey"
	_, err := c.call(ctx, "tools/call", map[string]interface{}{
		toolName: "addCommentToJiraIssue",
		toolArguments: map[string]interface{}{
			cloudID:       c.cloudID,
			issueKey:      ticketKey,
			"commentBody": body,
		},
	})
	return err
}

// adfDocument wraps plain text in an Atlassian Document Format document.
func adfDocument(text string) map[string]any {
	const nodeType, textNode = "type", "text"
	paragraphs := []any{}
	for _, line := range strings.Split(text, "\n") {
		paragraph := map[string]any{nodeType: "paragraph"}
		if line != "" {
			paragraph["content"] = []any{map[string]any{nodeType: textNode, textNode: line}}
		}
		paragraphs = append(paragraphs, paragraph)
	}
	return map[string]any{nodeType: "doc", "version": 1, "content": paragraphs}
}

// AddCommentForWorkspace comments on a ticket with one workspace's client.
func (s *Service) AddCommentForWorkspace(ctx context.Context, workspaceID, ticketKey, body string) error {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return err
	}
	commenter, ok := client.(IssueCommenter)
	if !ok {
		return ErrCommentUnsupported
	}
	return commenter.AddComment(ctx, ticketKey, body)
}

// TransitionMatch is the transition chosen to reach a status category.
// Current means the ticket already sits there; Available lists every
// transition when none qualifies.
type TransitionMatch struct {
	Transition *JiraTransition
	Current    bool
	Available  []string
}

// TransitionToCategoryForWorkspace moves a ticket to a status in category
// ("new", "indeterminate" or "done"). A transition whose name or target
// contains prefer ranks first; with strict, only such a transition qualifies
// and only such a status counts as already reached.
func (s *Service) TransitionToCategoryForWorkspace(ctx context.Context, workspaceID, ticketKey, category, prefer string, strict bool) (TransitionMatch, error) {
	workspaceID, err := s.normalizeWorkspaceID(workspaceID)
	if err != nil {
		return TransitionMatch{}, err
	}
	client, err := s.clientFor(ctx, workspaceID)
	if err != nil {
		return TransitionMatch{}, err
	}
	ticket, err := client.GetTicket(ctx, ticketKey)
	if err != nil {
		return TransitionMatch{}, err
	}
	if ticket.StatusCategory == category && (!strict || strings.Contains(strings.ToLower(ticket.StatusName), prefer)) {
		return TransitionMatch{Current: true}, nil
	}
	transitions, err := client.ListTransitions(ctx, ticketKey)
	if err != nil {
		return TransitionMatch{}, err
	}
	statuses, err := client.ListProjectStatuses(ctx, ticket.ProjectKey)
	if err != nil {
		return TransitionMatch{}, err
	}
	match := matchTransition(transitions, statuses, category, prefer, strict)
	if match.Transition == nil {
		return match, nil
	}
	return match, client.DoTransition(ctx, ticketKey, match.Transition.ID)
}

func matchTransition(transitions []JiraTransition, statuses []JiraStatus, category, prefer string, strict bool) TransitionMatch {
	categories := make(map[string]string, len(statuses))
	for _, status := range statuses {
		categories[status.ID] = status.StatusCategory
	}
	match := TransitionMatch{}
	preferred := false
	for i := range transitions {
		transition := &transitions[i]
		match.Available = append(match.Available, transition.Name+" (to "+transition.ToStatusName+")")
		if categories[transition.ToStatusID] != category {
			continue
		}
		mentions := prefer != "" && strings.Contains(strings.ToLower(transition.Name+" "+transition.ToStatusName), prefer)
		if (match.Transition == nil && !strict) || (mentions && !preferred) {
			match.Transition = transition
			preferred = mentions
		}
	}
	return match
}
