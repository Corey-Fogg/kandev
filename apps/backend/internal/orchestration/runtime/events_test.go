package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestration/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// claimedTurn queues and claims one coordinator turn bound to session.
func claimedTurn(t *testing.T, s *Service, task, key string) string {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, "chief", task, commentReason, key, nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, workspaceCoordinatorAudience, run.Payload, "session"))
	require.NoError(t, s.Repo.SetRuntimeWorking(ctx, "chief", true))
	return run.ID
}

func TestFinishTurnSettlesOnlyTheClaimedTurn(t *testing.T) {
	for name, tc := range map[string]struct {
		eventType      string
		data           func(task, run string) map[string]any
		handlerOwnsErr bool
		early          bool
		want           string
		note           bool
	}{
		"completed":     {eventType: events.AgentCompleted, want: "finished"},
		"stopped":       {eventType: events.AgentStopped, want: "finished"},
		"failed":        {eventType: events.AgentFailed, want: statusFailed, note: true},
		"failure owned": {eventType: events.AgentFailed, handlerOwnsErr: true, want: statusClaimed},
		"other session": {eventType: events.AgentCompleted, data: func(task, run string) map[string]any {
			return map[string]any{"task_id": task, "session_id": "other", "run_id": run}
		}, want: statusClaimed},
		"other run": {eventType: events.AgentCompleted, data: func(task, _ string) map[string]any {
			return map[string]any{"task_id": task, "session_id": "session", "run_id": "old"}
		}, want: statusClaimed},
		"before claim":      {eventType: events.AgentCompleted, early: true, want: statusClaimed},
		"not a turn ending": {eventType: events.TaskUpdated, want: statusClaimed},
	} {
		t.Run(name, func(t *testing.T) {
			s, _, task := newRuntime(t)
			ctx := context.Background()
			s.FailureHandlerInstalled = tc.handlerOwnsErr
			run := claimedTurn(t, s, task, "turn")
			data := map[string]any{"task_id": task, "session_id": "session", "run_id": run, "error_message": "Invalid API key; please log in"}
			if tc.data != nil {
				data = tc.data(task, run)
			}
			event := bus.NewEvent(tc.eventType, "test", data)
			if tc.early {
				event.Timestamp = time.Now().Add(-time.Hour)
			}
			require.NoError(t, s.onEvent(ctx, event))
			row, err := s.Runs.GetRunByID(ctx, run)
			require.NoError(t, err)
			require.EqualValues(t, tc.want, row.Status)
			persona, err := s.Personas.GetAgentInstance(ctx, "chief")
			require.NoError(t, err)
			require.Equal(t, tc.want == statusClaimed, persona.Status == models.AgentStatusWorking, "the coordinator stays working only while its turn is claimed")
			require.Equal(t, tc.note, len(runtimeNotes(t, s, task)) == 1)
		})
	}
}

func TestBridgeReplyRecordsEachReplyOnce(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	tasks := s.Tasks.(*testTasks)
	run := claimedTurn(t, s, task, "bridge")

	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentTurnMessageSaved, "test", map[string]any{"task_id": task, "session_id": "session"})))
	rows, err := s.Repo.ListComments(ctx, task, 10)
	require.NoError(t, err)
	require.Empty(t, rows, "an empty reply records nothing")

	tasks.text = "Created the delivery task."
	for range 2 {
		require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentTurnMessageSaved, "test", map[string]any{"task_id": task, "session_id": "session", "run_id": run})))
	}
	rows, err = s.Repo.ListComments(ctx, task, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1, "a turnless reply is keyed by the claimed run, not the event")
	require.Equal(t, "chief", rows[0].AuthorID)
	require.Equal(t, authorTypeAgent, rows[0].AuthorType)

	delegate(s, "delegated", v1.TaskStateInProgress)
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentTurnMessageSaved, "test", map[string]any{"task_id": "delegated", "session_id": "worker", "turn_id": "turn"})))
	rows, err = s.Repo.ListComments(ctx, "delegated", 10)
	require.NoError(t, err)
	require.Empty(t, rows, "replies outside a coordinator conversation are not bridged")
}
