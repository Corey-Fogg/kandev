package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
)

// Pull request states, pending actions and tracker status names used by the
// coordinator task projections.
const (
	pullRequestOpen    = "open"
	pullRequestMerged  = "merged"
	pendingPermission  = "permission"
	pendingQuestion    = "question"
	linearStateStarted = "started"
	jiraInProgress     = "indeterminate"
)

// taskPullRequestLookup returns the most relevant pull request per task id.
type taskPullRequestLookup func(context.Context, []string) (map[string]shared.TaskPullRequest, error)

// githubTaskPullRequests reads task pull requests from the GitHub store, or
// returns nil when GitHub is unavailable.
func githubTaskPullRequests(gh *github.Service) taskPullRequestLookup {
	if gh == nil {
		return nil
	}
	return func(ctx context.Context, taskIDs []string) (map[string]shared.TaskPullRequest, error) {
		result := map[string]shared.TaskPullRequest{}
		if len(taskIDs) == 0 {
			return result, nil
		}
		byTask, err := gh.ListTaskPRsByTaskIDs(ctx, taskIDs)
		if err != nil {
			return nil, err
		}
		for taskID, prs := range byTask {
			if pr := preferredPullRequest(prs); pr != nil {
				result[taskID] = shared.TaskPullRequest{Number: pr.PRNumber, URL: pr.PRURL, State: pr.State}
			}
		}
		return result, nil
	}
}

// preferredPullRequest picks an open pull request over a merged one over a
// closed one, and the highest number within a state.
func preferredPullRequest(prs []*github.TaskPR) *github.TaskPR {
	rank := func(state string) int {
		switch state {
		case pullRequestOpen:
			return 3
		case pullRequestMerged:
			return 2
		}
		return 1
	}
	var best *github.TaskPR
	for _, pr := range prs {
		if pr == nil {
			continue
		}
		if best == nil || rank(pr.State) > rank(best.State) || (rank(pr.State) == rank(best.State) && pr.PRNumber > best.PRNumber) {
			best = pr
		}
	}
	return best
}

// pendingActions names the input each task's sessions wait on; a permission
// outranks a question.
func (a *taskCreatorAdapter) pendingActions(ctx context.Context, taskIDs []string) (map[string]string, error) {
	result := map[string]string{}
	if len(taskIDs) == 0 {
		return result, nil
	}
	pending, err := a.taskSvc.ListPendingInteractions(ctx, models.PendingInteractionFilter{TaskIDs: taskIDs})
	if err != nil {
		return nil, err
	}
	for _, interaction := range pending {
		if interaction.Kind == models.InteractionKindPermission {
			result[interaction.TaskID] = pendingPermission
			continue
		}
		if result[interaction.TaskID] == "" {
			result[interaction.TaskID] = pendingQuestion
		}
	}
	return result, nil
}

// enrichTaskSummaries adds pull requests and pending input to summary rows.
func (a *taskCreatorAdapter) enrichTaskSummaries(ctx context.Context, rows []shared.WorkspaceTaskSummary) error {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	actions, err := a.pendingActions(ctx, ids)
	if err != nil {
		return err
	}
	prs := map[string]shared.TaskPullRequest{}
	if a.pullRequests != nil {
		if prs, err = a.pullRequests(ctx, ids); err != nil {
			return err
		}
	}
	for i := range rows {
		rows[i].PendingAction = actions[rows[i].ID]
		if pr, ok := prs[rows[i].ID]; ok {
			rows[i].PullRequest = &pr
		}
	}
	return nil
}

// sourceIssueWriter writes coordinator updates back to the Jira or Linear
// issue recorded on a workspace task.
type sourceIssueWriter struct {
	tasks interface {
		GetTask(context.Context, string) (*models.Task, error)
	}
	jira   *jira.Service
	linear *linear.Service
}

// sourceStateTargets maps a write-back state to a tracker status category, a
// name hint, and whether only a status matching the hint qualifies.
var sourceStateTargets = map[string]struct {
	jiraCategory, linearType, prefer string
	strict                           bool
}{
	shared.SourceStateStarted: {jiraInProgress, linearStateStarted, "progress", false},
	shared.SourceStateReview:  {jiraInProgress, linearStateStarted, "review", true},
	shared.SourceStateDone:    {"done", "completed", "", false},
}

func (w sourceIssueWriter) UpdateSourceIssue(ctx context.Context, workspaceID, taskID string, update shared.SourceIssueUpdate) (shared.SourceIssueResult, error) {
	task, err := w.tasks.GetTask(ctx, taskID)
	if err != nil || task.WorkspaceID != workspaceID {
		return shared.SourceIssueResult{}, fmt.Errorf("task must belong to this workspace")
	}
	source := shared.TaskSourceIssue(task.Metadata)
	if source == nil {
		return shared.SourceIssueResult{}, shared.ErrNoSourceIssue
	}
	result := shared.SourceIssueResult{Source: *source}
	if (source.Tracker == shared.TrackerJira && w.jira == nil) || (source.Tracker == shared.TrackerLinear && w.linear == nil) {
		return result, fmt.Errorf("%s integration is unavailable", source.Tracker)
	}
	if update.Comment != "" {
		if err := w.comment(ctx, workspaceID, *source, update.Comment); err != nil {
			return result, err
		}
		result.Commented = true
	}
	if update.State == "" {
		return result, nil
	}
	return w.move(ctx, workspaceID, *source, update.State, result)
}

func (w sourceIssueWriter) comment(ctx context.Context, workspaceID string, source shared.SourceIssue, body string) error {
	if source.Tracker == shared.TrackerLinear {
		return w.linear.AddCommentForWorkspace(ctx, workspaceID, source.Key, body)
	}
	return w.jira.AddCommentForWorkspace(ctx, workspaceID, source.Key, body)
}

func (w sourceIssueWriter) move(ctx context.Context, workspaceID string, source shared.SourceIssue, state string, result shared.SourceIssueResult) (shared.SourceIssueResult, error) {
	target, ok := sourceStateTargets[state]
	if !ok {
		return result, fmt.Errorf("state must be started, review or done")
	}
	if source.Tracker == shared.TrackerLinear {
		match, err := w.linear.MoveToStateTypeForWorkspace(ctx, workspaceID, source.Key, target.linearType, target.prefer, target.strict)
		if err != nil {
			return result, err
		}
		result.Unchanged = match.Current
		if match.State != nil {
			result.State = match.State.Name
		} else if !match.Current {
			return result, &shared.SourceStateError{State: state, Available: match.Available}
		}
		return result, nil
	}
	match, err := w.jira.TransitionToCategoryForWorkspace(ctx, workspaceID, source.Key, target.jiraCategory, target.prefer, target.strict)
	if err != nil {
		return result, err
	}
	result.Unchanged = match.Current
	if match.Transition != nil {
		result.State = match.Transition.ToStatusName
	} else if !match.Current {
		return result, &shared.SourceStateError{State: state, Available: match.Available}
	}
	return result, nil
}
