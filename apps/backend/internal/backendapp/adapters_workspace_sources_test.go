package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestWorkspaceTaskSourceRecordsIssueAndReportsDuplicate(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Delivery"})
	require.NoError(t, err)
	source := &shared.SourceIssue{Tracker: shared.TrackerLinear, Key: "ENG-7", URL: "https://linear.app/example/issue/ENG-7"}
	spec := shared.WorkspaceTaskSpec{WorkspaceID: "ws-1", ChiefID: "chief", WorkflowID: wf.ID, Title: "Fix login", Source: source}
	id, err := adapter.CreateWorkspaceTask(ctx, spec)
	require.NoError(t, err)
	task, err := svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "ENG-7", task.Metadata[shared.MetaLinearIssueIdentifier])
	require.Equal(t, "https://linear.app/example/issue/ENG-7", task.Metadata[shared.MetaLinearIssueURL])
	require.Equal(t, "linear:ENG-7", task.ExternalID)
	require.NotNil(t, task.ExternalIDSettledAt, "the identity is settled once the task exists")
	require.Equal(t, source, shared.TaskSourceIssue(task.Metadata))

	_, err = adapter.CreateWorkspaceTask(ctx, spec)
	var duplicate *shared.DuplicateTaskError
	require.True(t, errors.As(err, &duplicate), err)
	require.Equal(t, id, duplicate.TaskID)
}

func TestWorkspaceTaskSummariesCarrySourcePullRequestAndPendingAction(t *testing.T) {
	adapter, _ := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	task := seedDelegatedQuestion(t, adapter, "chief")
	task.Metadata[shared.MetaJiraIssueKey] = "ABC-1"
	task.Metadata[shared.MetaJiraIssueURL] = "javascript:alert(1)"
	task.ParentID = "parent"
	row := workspaceTaskSummary(task)
	require.Equal(t, &shared.SourceIssue{Tracker: shared.TrackerJira, Key: "ABC-1"}, row.Source, "an unsafe issue URL is dropped")
	require.Equal(t, "parent", row.ParentID)

	adapter.pullRequests = func(_ context.Context, ids []string) (map[string]shared.TaskPullRequest, error) {
		require.Equal(t, []string{"delegated", "idle"}, ids)
		return map[string]shared.TaskPullRequest{"delegated": {Number: 3, URL: "https://github.com/example/repo/pull/3", State: "open"}}, nil
	}
	rows := []shared.WorkspaceTaskSummary{row, {ID: "idle"}}
	require.NoError(t, adapter.enrichTaskSummaries(ctx, rows))
	require.Equal(t, "question", rows[0].PendingAction)
	require.Equal(t, &shared.TaskPullRequest{Number: 3, URL: "https://github.com/example/repo/pull/3", State: "open"}, rows[0].PullRequest)
	require.Empty(t, rows[1].PendingAction)
	require.Nil(t, rows[1].PullRequest)
}

func TestPreferredPullRequestRanksOpenOverMergedOverClosed(t *testing.T) {
	prs := []*github.TaskPR{{PRNumber: 9, State: "closed"}, {PRNumber: 4, State: "merged"}, {PRNumber: 5, State: "merged"}, nil}
	require.Equal(t, 5, preferredPullRequest(prs).PRNumber)
	prs = append(prs, &github.TaskPR{PRNumber: 1, State: "open"})
	require.Equal(t, 1, preferredPullRequest(prs).PRNumber)
	require.Nil(t, preferredPullRequest(nil))
}

type sourceTaskReader map[string]*models.Task

func (r sourceTaskReader) GetTask(_ context.Context, id string) (*models.Task, error) {
	if task := r[id]; task != nil {
		return task, nil
	}
	return nil, errors.New("not found")
}

func TestSourceIssueWriterResolvesIssueFromTaskOnly(t *testing.T) {
	ctx := context.Background()
	writer := sourceIssueWriter{tasks: sourceTaskReader{
		"plain":   {ID: "plain", WorkspaceID: "ws"},
		"jira":    {ID: "jira", WorkspaceID: "ws", Metadata: map[string]any{shared.MetaJiraIssueKey: "ABC-1"}},
		"foreign": {ID: "foreign", WorkspaceID: "other", Metadata: map[string]any{shared.MetaJiraIssueKey: "ABC-2"}},
	}}
	update := shared.SourceIssueUpdate{Comment: "Done."}
	_, err := writer.UpdateSourceIssue(ctx, "ws", "foreign", update)
	require.Error(t, err)
	_, err = writer.UpdateSourceIssue(ctx, "ws", "plain", update)
	require.ErrorIs(t, err, shared.ErrNoSourceIssue)
	result, err := writer.UpdateSourceIssue(ctx, "ws", "jira", update)
	require.ErrorContains(t, err, "jira integration is unavailable")
	require.Equal(t, shared.SourceIssue{Tracker: shared.TrackerJira, Key: "ABC-1"}, result.Source)
	require.False(t, result.Commented)
	for _, state := range []string{shared.SourceStateStarted, shared.SourceStateReview, shared.SourceStateDone} {
		require.Contains(t, sourceStateTargets, state)
	}
}
