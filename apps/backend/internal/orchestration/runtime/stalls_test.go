package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func queuedUpdates(t *testing.T, s *Service, id string) []taskUpdate {
	t.Helper()
	run, err := s.Runs.GetRunByID(context.Background(), id)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(run.Payload), &payload))
	return payloadUpdates(payload)
}

func TestStallEventsWakeCoordinatorWithOutcome(t *testing.T) {
	agentStall := func(never bool, generation int) map[string]any {
		return map[string]any{"task_id": "delegated", "session_id": "worker", "prompt_generation": generation, "stalled_for": 10 * time.Minute, "never_started": never}
	}
	for _, tc := range []struct {
		name, subject, outcome, stalledFor string
		data                               map[string]any
	}{
		{"no progress", events.AgentStalled, stallNoProgress, "10m0s", agentStall(false, 1)},
		{"never started", events.AgentStalled, stallNeverStarted, "10m0s", agentStall(true, 1)},
		{"orphaned", events.TaskStalled, stallOrphaned, "2h0m1s", map[string]any{"task_id": "delegated", "session_ids": []string{"worker"}, "stalled_for": "2h0m0.6s", "last_event_at": "2026-09-23T10:00:00Z"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, db, _ := newRuntime(t)
			ctx := context.Background()
			delegate(s, "delegated", v1.TaskStateInProgress)
			require.NoError(t, s.onEvent(ctx, bus.NewEvent(tc.subject, "test", tc.data)))
			require.NoError(t, s.onEvent(ctx, bus.NewEvent(tc.subject, "test", tc.data)))
			ids := queuedCallbacks(t, db)
			require.Len(t, ids, 1, "one stall episode wakes the coordinator once")
			updates := queuedUpdates(t, s, ids[0])
			require.Len(t, updates, 1)
			require.Equal(t, tc.outcome, updates[0].StallOutcome)
			require.Equal(t, tc.stalledFor, updates[0].StalledFor)
			require.Equal(t, "IN_PROGRESS", updates[0].State)
		})
	}
}

func TestNewStallEpisodeWakesAgainAndUndelegatedStallsDoNot(t *testing.T) {
	s, db, conversation := newRuntime(t)
	ctx := context.Background()
	delegate(s, "delegated", v1.TaskStateInProgress)
	for _, generation := range []int{1, 2} {
		data := map[string]any{"task_id": "delegated", "session_id": "worker", "prompt_generation": generation}
		require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentStalled, "test", data)))
	}
	require.Len(t, queuedCallbacks(t, db), 2)
	delegate(s, conversation, v1.TaskStateInProgress)
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentStalled, "test", map[string]any{"task_id": conversation, "session_id": "own"})))
	require.Len(t, queuedCallbacks(t, db), 2, "the coordinator's own conversation never wakes itself")
}

func TestTaskUpdateCarriesSourceIssueAndPullRequest(t *testing.T) {
	s, db, _ := newRuntime(t)
	ctx := context.Background()
	task := delegate(s, "delegated", v1.TaskStateReview)
	task.Metadata[models.MetaJiraIssueKey] = "ABC-12"
	task.Metadata[models.MetaJiraIssueURL] = "https://example.atlassian.net/browse/ABC-12"
	pr := models.TaskPullRequest{Number: 7, URL: "https://github.com/example/repo/pull/7", State: "open"}
	s.PullRequests = func(_ context.Context, ids []string) (map[string]models.TaskPullRequest, error) {
		require.Equal(t, []string{"delegated"}, ids)
		return map[string]models.TaskPullRequest{"delegated": pr}, nil
	}
	require.NoError(t, s.taskCallback(ctx, "delegated"))
	ids := queuedCallbacks(t, db)
	require.Len(t, ids, 1)
	updates := queuedUpdates(t, s, ids[0])
	require.Equal(t, &models.SourceIssue{Tracker: models.TrackerJira, Key: "ABC-12", URL: "https://example.atlassian.net/browse/ABC-12"}, updates[0].Source)
	require.Equal(t, &pr, updates[0].PullRequest)

	pr.State = "merged"
	require.NoError(t, s.taskCallback(ctx, "delegated"))
	require.Len(t, queuedCallbacks(t, db), 2, "a pull request change is a new update")

	var text strings.Builder
	writeTaskUpdates(&text, []taskUpdate{{TaskID: "delegated", Title: "Delegated", State: "REVIEW", Source: updates[0].Source, PullRequest: &pr, StallOutcome: stallNoProgress, StalledFor: "10m0s"}})
	require.Contains(t, text.String(), "Stalled: no_progress after 10m0s")
	require.Contains(t, text.String(), "Source issue: jira ABC-12 https://example.atlassian.net/browse/ABC-12")
	require.Contains(t, text.String(), "Pull request: #7 merged https://github.com/example/repo/pull/7")
}

// A delegated session that stopped on a login refresh failure tells its
// coordinator to call repair_session, even while the task stays in progress.
func TestLoginRefreshFailureDigestAsksForRepairSession(t *testing.T) {
	s, db, _ := newRuntime(t)
	ctx := context.Background()
	delegate(s, "delegated", v1.TaskStateInProgress)
	s.Tasks.(*testTasks).sessions = []*taskmodels.TaskSession{{ID: "worker", TaskID: "delegated", State: taskmodels.TaskSessionStateFailed,
		ErrorMessage: "Internal error: Failed to refresh OAuth token: *** Claude Code process is refreshing it or exited mid-refresh"}}
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.TaskStateChanged, "test", map[string]string{"task_id": "delegated"})))
	ids := queuedCallbacks(t, db)
	require.Len(t, ids, 1)
	updates := queuedUpdates(t, s, ids[0])
	require.Len(t, updates, 1)
	require.True(t, updates[0].LoginFailure)
	var text strings.Builder
	writeTaskUpdates(&text, updates)
	require.Contains(t, text.String(), "login refresh failure — call repair_session")

	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentStalled, "test", map[string]any{"task_id": "delegated", "session_id": "worker", "prompt_generation": 1})))
	ids = queuedCallbacks(t, db)
	stall := queuedUpdates(t, s, ids[len(ids)-1])
	require.True(t, stall[len(stall)-1].LoginFailure, "the stall digest carries the hint too")

	var plain strings.Builder
	writeTaskUpdates(&plain, []taskUpdate{{TaskID: "other", Title: "Other", State: "FAILED", Error: "git push rejected"}})
	require.NotContains(t, plain.String(), "repair_session")
}
