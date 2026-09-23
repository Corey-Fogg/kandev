package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	settings "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestration/models"
	store "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// writeBackRuntime is a runtime whose delegated task "sourced" records a Jira
// issue, with a synchronous write-back runner.
func writeBackRuntime(t *testing.T, s *Service, db *sqlx.DB, move bool) *fakeSourceIssues {
	t.Helper()
	// The write-back ledger references the stored task.
	_, err := db.Exec(`INSERT INTO tasks(id,workspace_id,title,created_at,updated_at) VALUES('sourced','ws','Delegated sourced',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	writer := &fakeSourceIssues{result: models.SourceIssueResult{Commented: true}}
	s.SourceIssues = writer
	s.WriteBackRunner = func(job func()) { job() }
	settings := models.DefaultOrchestratorSettings()
	settings.AutoMoveSourceDone = move
	require.NoError(t, s.Repo.SaveOrchestratorSettings(context.Background(), "chief", settings))
	task := delegate(s, "sourced", v1.TaskStateInProgress)
	task.Metadata[models.MetaJiraIssueKey] = "ABC-12"
	return writer
}

func changeState(t *testing.T, s *Service, id string, state v1.TaskState) {
	t.Helper()
	task := s.Tasks.(*testTasks).tasks[id]
	previous := task.State
	task.State = state
	data := map[string]string{"task_id": id, "old_state": string(previous), "new_state": string(state)}
	require.NoError(t, s.onEvent(context.Background(), bus.NewEvent(events.TaskStateChanged, "test", data)))
}

// moveTask publishes a task.moved event, which carries no state change.
func moveTask(t *testing.T, s *Service, id string) {
	t.Helper()
	require.NoError(t, s.onEvent(context.Background(), bus.NewEvent(events.TaskMoved, "test", map[string]string{"task_id": id})))
}

func TestReviewCommentsOncePerTransition(t *testing.T) {
	s, db, _ := newRuntime(t)
	writer := writeBackRuntime(t, s, db, false)
	s.PullRequests = func(context.Context, []string) (map[string]models.TaskPullRequest, error) {
		return map[string]models.TaskPullRequest{"sourced": {Number: 7, URL: "https://github.com/example/repo/pull/7", State: "open"}}, nil
	}
	changeState(t, s, "sourced", v1.TaskStateInProgress)
	require.Empty(t, writer.calls)
	changeState(t, s, "sourced", v1.TaskStateReview)
	require.Len(t, writer.calls, 1)
	require.Equal(t, "Kandev: \"Delegated sourced\" is ready for review.\nPull request: https://github.com/example/repo/pull/7", writer.calls[0].Comment)
	require.Empty(t, writer.calls[0].State)
	changeState(t, s, "sourced", v1.TaskStateReview)
	require.Len(t, writer.calls, 1, "a redelivered event writes nothing")
	changeState(t, s, "sourced", v1.TaskStateInProgress)
	changeState(t, s, "sourced", v1.TaskStateReview)
	require.Len(t, writer.calls, 2, "a second review is a new transition")
	row, err := s.Repo.GetSourceWriteBack(context.Background(), "sourced")
	require.NoError(t, err)
	require.Equal(t, store.WriteBackPosted, row.Status)
	require.True(t, row.Commented)
}

func TestCompletionMovesTheIssueOnlyWhenEnabled(t *testing.T) {
	for _, move := range []bool{true, false} {
		s, db, _ := newRuntime(t)
		writer := writeBackRuntime(t, s, db, move)
		task := s.Tasks.(*testTasks).tasks["sourced"]
		goal, err := models.NewTaskGoal([]string{"Tests pass"}, s.now())
		require.NoError(t, err)
		task.Metadata[models.MetaTaskGoal] = goal
		changeState(t, s, "sourced", v1.TaskStateCompleted)
		require.Len(t, writer.calls, 1)
		require.Equal(t, "Kandev: \"Delegated sourced\" is complete.\nAcceptance criteria: 0 of 1 met.", writer.calls[0].Comment)
		want := ""
		if move {
			want = models.SourceStateDone
		}
		require.Equal(t, want, writer.calls[0].State)
	}
}

func TestWriteBackUsesTheDelegatingOrchestratorsSettings(t *testing.T) {
	s, db, _ := newRuntime(t)
	writer := writeBackRuntime(t, s, db, false)
	registerSecondOrchestrator(t, s)
	require.NoError(t, s.Repo.SaveOrchestratorSettings(context.Background(), "second",
		models.OrchestratorSettings{AutoCommentSource: true, AutoMoveSourceDone: true}))
	s.Tasks.(*testTasks).tasks["sourced"].Metadata["orchestration_chief_id"] = "second"
	changeState(t, s, "sourced", v1.TaskStateCompleted)
	require.Len(t, writer.calls, 1)
	require.Equal(t, models.SourceStateDone, writer.calls[0].State, "the owner's move setting applies, not the workspace's other orchestrator's")
}

func TestDisabledWriteBackRecordsStateWithoutPosting(t *testing.T) {
	s, db, _ := newRuntime(t)
	writer := writeBackRuntime(t, s, db, false)
	require.NoError(t, s.Repo.SaveOrchestratorSettings(context.Background(), "chief", models.OrchestratorSettings{}))
	changeState(t, s, "sourced", v1.TaskStateReview)
	require.Empty(t, writer.calls)
	require.NoError(t, s.Repo.SaveOrchestratorSettings(context.Background(), "chief", models.DefaultOrchestratorSettings()))
	changeState(t, s, "sourced", v1.TaskStateReview)
	require.Empty(t, writer.calls, "enabling comments later never posts a past transition")
}

func TestWriteBackSkipsUnsourcedUndelegatedAndPausedTasks(t *testing.T) {
	s, db, conversation := newRuntime(t)
	writer := writeBackRuntime(t, s, db, true)
	delegate(s, "plain", v1.TaskStateInProgress)
	changeState(t, s, "plain", v1.TaskStateCompleted)
	undelegated := delegate(s, "undelegated", v1.TaskStateInProgress)
	undelegated.Metadata = map[string]any{models.MetaJiraIssueKey: "ABC-13"}
	changeState(t, s, "undelegated", v1.TaskStateCompleted)
	own := delegate(s, conversation, v1.TaskStateInProgress)
	own.Metadata[models.MetaJiraIssueKey] = "ABC-14"
	changeState(t, s, conversation, v1.TaskStateCompleted)
	_, err := s.Personas.UpdateAgentStatus(context.Background(), "chief", settings.AgentStatusPaused, "")
	require.NoError(t, err)
	s.Tasks.(*testTasks).tasks["sourced"].State = v1.TaskStateCompleted
	require.NoError(t, s.observeSourceWriteBack(context.Background(), "sourced", true))
	require.Empty(t, writer.calls)
	_, err = s.Repo.GetSourceWriteBack(context.Background(), "sourced")
	require.Error(t, err, "a paused coordinator records no state, so resuming it still reports the transition")
}

func TestFailedWriteBackWakesTheCoordinatorOnce(t *testing.T) {
	s, db, _ := newRuntime(t)
	writer := writeBackRuntime(t, s, db, false)
	writer.err = errors.New("jira returned 500")
	changeState(t, s, "sourced", v1.TaskStateReview)
	require.Len(t, writer.calls, 1)
	row, err := s.Repo.GetSourceWriteBack(context.Background(), "sourced")
	require.NoError(t, err)
	require.Equal(t, store.WriteBackFailed, row.Status)
	require.Equal(t, "jira returned 500", row.Error)
	var failures []taskUpdate
	for _, id := range queuedCallbacks(t, db) {
		for _, update := range queuedUpdates(t, s, id) {
			if update.SourceWriteBackError != "" {
				failures = append(failures, update)
			}
		}
	}
	require.Len(t, failures, 1)
	require.Equal(t, "jira returned 500", failures[0].SourceWriteBackError)
	require.True(t, failures[0].actionable())
}

func TestUnavailableIntegrationIsSkippedSilently(t *testing.T) {
	s, db, _ := newRuntime(t)
	writer := writeBackRuntime(t, s, db, false)
	writer.err = errors.New("jira integration is unavailable")
	changeState(t, s, "sourced", v1.TaskStateReview)
	row, err := s.Repo.GetSourceWriteBack(context.Background(), "sourced")
	require.NoError(t, err)
	require.Equal(t, store.WriteBackSkipped, row.Status)
	for _, id := range queuedCallbacks(t, db) {
		for _, update := range queuedUpdates(t, s, id) {
			require.Empty(t, update.SourceWriteBackError)
		}
	}
}

func TestMissingWriterSkipsTheClaimedWriteBack(t *testing.T) {
	s, db, _ := newRuntime(t)
	writeBackRuntime(t, s, db, false)
	s.SourceIssues = nil
	changeState(t, s, "sourced", v1.TaskStateReview)
	row, err := s.Repo.GetSourceWriteBack(context.Background(), "sourced")
	require.NoError(t, err)
	require.Equal(t, store.WriteBackSkipped, row.Status)
}

func TestTaskAlreadyInReviewWhenFirstSeenPostsNothing(t *testing.T) {
	s, db, _ := newRuntime(t)
	writer := writeBackRuntime(t, s, db, true)
	// The task reached review before the ledger existed, so no row records it.
	s.Tasks.(*testTasks).tasks["sourced"].State = v1.TaskStateCompleted
	moveTask(t, s, "sourced")
	require.Empty(t, writer.calls, "a reorder of a task already complete is not a transition")
	row, err := s.Repo.GetSourceWriteBack(context.Background(), "sourced")
	require.NoError(t, err)
	require.Equal(t, string(v1.TaskStateCompleted), row.LastState)
	changeState(t, s, "sourced", v1.TaskStateInProgress)
	changeState(t, s, "sourced", v1.TaskStateReview)
	require.Len(t, writer.calls, 1, "a later real transition still posts")
}

// hangingSourceIssues blocks until its context ends, as a tracker that never
// answers does.
type hangingSourceIssues struct{}

func (hangingSourceIssues) UpdateSourceIssue(ctx context.Context, _, _ string, _ models.SourceIssueUpdate) (models.SourceIssueResult, error) {
	<-ctx.Done()
	return models.SourceIssueResult{}, ctx.Err()
}

func TestTrackerTimeoutStillRecordsTheFailureAndWakesTheCoordinator(t *testing.T) {
	s, db, _ := newRuntime(t)
	writeBackRuntime(t, s, db, false)
	s.SourceIssues = hangingSourceIssues{}
	previous := sourceWriteBackTimeout
	sourceWriteBackTimeout = 20 * time.Millisecond
	t.Cleanup(func() { sourceWriteBackTimeout = previous })
	changeState(t, s, "sourced", v1.TaskStateReview)
	row, err := s.Repo.GetSourceWriteBack(context.Background(), "sourced")
	require.NoError(t, err)
	require.Equal(t, store.WriteBackFailed, row.Status, "the ledger records a timed-out tracker call")
	require.Contains(t, row.Error, "deadline exceeded")
	var failures int
	for _, id := range queuedCallbacks(t, db) {
		for _, update := range queuedUpdates(t, s, id) {
			if update.SourceWriteBackError != "" {
				failures++
			}
		}
	}
	require.Equal(t, 1, failures, "the coordinator is woken after a tracker timeout")
}
