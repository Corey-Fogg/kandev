package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func delegate(s *Service, id string, state v1.TaskState) *taskmodels.Task {
	task := &taskmodels.Task{ID: id, WorkspaceID: "ws", Title: "Delegated " + id, State: state, Metadata: map[string]any{"orchestration_chief_id": "chief"}}
	s.Tasks.(*testTasks).tasks[id] = task
	return task
}

func queuedCallbacks(t *testing.T, db *sqlx.DB) []string {
	t.Helper()
	var ids []string
	require.NoError(t, db.Select(&ids, `SELECT id FROM runs WHERE reason=? AND status='queued' ORDER BY requested_at, id`, callbackReason))
	return ids
}

func TestTaskCallbackWakesOnlyForReportableStates(t *testing.T) {
	for state, wakes := range map[v1.TaskState]bool{
		v1.TaskStateReview: true, v1.TaskStateCompleted: true, v1.TaskStateFailed: true,
		v1.TaskStateWaitingForInput: true, v1.TaskStateBlocked: true,
		v1.TaskStateInProgress: false, v1.TaskStateTODO: false,
	} {
		t.Run(string(state), func(t *testing.T) {
			s, db, _ := newRuntime(t)
			delegate(s, "delegated", state)
			require.NoError(t, s.onEvent(context.Background(), bus.NewEvent(events.TaskStateChanged, "test", map[string]string{"task_id": "delegated"})))
			require.Equal(t, wakes, len(queuedCallbacks(t, db)) == 1)
		})
	}
}

func TestTaskCallbackWakesForATaskWhoseSessionHasNotReplied(t *testing.T) {
	s, db, _ := newRuntime(t)
	delegate(s, "delegated", v1.TaskStateCompleted)
	tasks := s.Tasks.(*testTasks)
	tasks.sessions = []*taskmodels.TaskSession{{ID: "silent", TaskID: "delegated", State: taskmodels.TaskSessionStateCompleted}}
	tasks.textErr = sql.ErrNoRows
	require.NoError(t, s.taskCallback(context.Background(), "delegated"))
	require.Len(t, queuedCallbacks(t, db), 1)
}

func TestTaskCallbackSkipsConversationAndUndelegatedTasks(t *testing.T) {
	s, db, conversation := newRuntime(t)
	ctx := context.Background()
	delegate(s, conversation, v1.TaskStateReview)
	s.Tasks.(*testTasks).tasks["plain"] = &taskmodels.Task{ID: "plain", WorkspaceID: "ws", State: v1.TaskStateReview}
	require.NoError(t, s.taskCallback(ctx, conversation))
	require.NoError(t, s.taskCallback(ctx, "plain"))
	require.Empty(t, queuedCallbacks(t, db))
}

func TestTaskCallbackSuppressesUnchangedDigest(t *testing.T) {
	s, db, _ := newRuntime(t)
	ctx := context.Background()
	delegate(s, "delegated", v1.TaskStateReview)
	tasks := s.Tasks.(*testTasks)
	tasks.sessions = []*taskmodels.TaskSession{{ID: "worker", TaskID: "delegated", State: taskmodels.TaskSessionStateWaitingForInput, UpdatedAt: time.Now()}}
	tasks.text = "Opened the pull request."
	require.NoError(t, s.taskCallback(ctx, "delegated"))
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.TaskMoved, "test", map[string]string{"task_id": "delegated"})))
	require.Len(t, queuedCallbacks(t, db), 1, "a repeat with nothing new is suppressed")
	tasks.text = "Addressed the review comments."
	require.NoError(t, s.taskCallback(ctx, "delegated"))
	require.Len(t, queuedCallbacks(t, db), 2, "a new reply is a new update")
}

func TestPendingInputWakesCoordinatorWithoutStateChange(t *testing.T) {
	s, db, _ := newRuntime(t)
	ctx := context.Background()
	delegate(s, "delegated", v1.TaskStateInProgress)
	event := bus.NewEvent(events.SessionPendingActionChanged, "test", map[string]any{"task_id": "delegated", "session_id": "worker", "pending_action": nil})
	require.NoError(t, s.onEvent(ctx, event))
	require.Empty(t, queuedCallbacks(t, db), "a cleared pending action is not a wake")
	s.Tasks.(*testTasks).pending = []*taskmodels.Interaction{
		{ID: "permission", Kind: taskmodels.InteractionKindPermission, TaskID: "delegated", SessionID: "worker", RequestID: "request"},
		{ID: "question", Kind: taskmodels.InteractionKindClarification, TaskID: "delegated", SessionID: "worker",
			Questions: []taskmodels.InteractionQuestion{{ID: "q1", Prompt: "Which base branch?"}}},
	}
	event.Data = map[string]any{"task_id": "delegated", "session_id": "worker", "pending_action": "permission"}
	require.NoError(t, s.onEvent(ctx, event))
	ids := queuedCallbacks(t, db)
	require.Len(t, ids, 1)
	run, err := s.Runs.GetRunByID(ctx, ids[0])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(run.Payload), &payload))
	updates := payloadUpdates(payload)
	require.Len(t, updates, 1)
	require.Equal(t, 1, updates[0].PendingPermissions)
	require.Equal(t, 1, updates[0].PendingQuestions)
	require.Equal(t, []string{"Which base branch?"}, updates[0].Questions)
}

func TestQueuedCallbacksCoalesceIntoOneTurn(t *testing.T) {
	s, db, _ := newRuntime(t)
	ctx := context.Background()
	tasks := s.Tasks.(*testTasks)
	for _, id := range []string{"first", "second"} {
		delegate(s, id, v1.TaskStateReview)
		require.NoError(t, s.taskCallback(ctx, id))
	}
	tasks.tasks["first"].State = v1.TaskStateCompleted
	require.NoError(t, s.taskCallback(ctx, "first"))
	require.Len(t, queuedCallbacks(t, db), 3)

	var prompt string
	s.Start = func(ctx context.Context, l Launch) error {
		prompt = l.Prompt
		return l.OnSessionPrepared(ctx, "session")
	}
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	handled, err := s.Process(ctx, run)
	require.True(t, handled)
	require.NoError(t, err)
	require.Empty(t, queuedCallbacks(t, db), "queued callbacks were delivered by the running turn")
	require.Contains(t, prompt, "Delegated first (task_id=first, state=COMPLETED")
	require.NotContains(t, prompt, "task_id=first, state=REVIEW", "only the latest update per task is shown")
	require.Contains(t, prompt, "Delegated second (task_id=second, state=REVIEW")
	stored, err := s.Runs.GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(stored.Payload), &payload))
	require.Len(t, payloadUpdates(payload), 2, "the merged updates survive a retry of the run")
	_, err = s.Runs.ClaimNextEligibleRun(ctx)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestSupersededMessageTurnIsSkippedAtLaunch(t *testing.T) {
	s, db, task := newRuntime(t)
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, "chief", task, commentReason, "older", map[string]any{intentRevisionKey: 1}))
	_, err := db.Exec(`INSERT INTO orchestration_conversation_intents(task_id,revision) VALUES(?,2)
		ON CONFLICT(task_id) DO UPDATE SET revision=2`, task)
	require.NoError(t, err)
	started := false
	s.Start = func(context.Context, Launch) error { started = true; return nil }
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	handled, err := s.Process(ctx, run)
	require.True(t, handled)
	require.NoError(t, err)
	require.False(t, started, "a superseded message never starts an agent turn")
	stored, err := s.Runs.GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, "finished", stored.Status)
	require.NotNil(t, stored.Outcome)
	require.Equal(t, supersededOutcome, *stored.Outcome)
}

func TestSupersededMessagesReachTheCoalescedTurn(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	router := conversationRouter(s)
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	send := func(body string) {
		require.Equal(t, 201, runtimeRequest(t, router, "POST", path, "", "", map[string]string{"body": body}).Code)
	}
	var prompts []string
	s.Start = func(ctx context.Context, l Launch) error {
		prompts = append(prompts, l.Prompt)
		return l.OnSessionPrepared(ctx, "session")
	}
	send("answered message")
	first, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	_, err = s.Process(ctx, first)
	require.NoError(t, err)
	_, err = s.Runs.FinishRun(ctx, first.ID, "finished", nil)
	require.NoError(t, err)

	for i := 1; i <= 6; i++ {
		send(fmt.Sprintf("queued message %d", i))
		require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{TaskID: task, AuthorType: "agent", AuthorID: "chief", Body: "note", Source: "agent"}))
	}
	for {
		run, err := s.Runs.ClaimNextEligibleRun(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		require.NoError(t, err)
		_, err = s.Process(ctx, run)
		require.NoError(t, err)
	}
	require.Len(t, prompts, 2, "only the newest message launches a turn")
	prompt := prompts[1]
	for i := 1; i <= 5; i++ {
		require.Contains(t, prompt, fmt.Sprintf("queued message %d", i), "superseded message %d reaches the coalesced turn", i)
	}
	require.Contains(t, prompt, "Current user message")
	require.NotContains(t, prompt, "): answered message", "a message whose turn completed is not repeated")
}

func TestUnansweredMessagesAreBounded(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	router := conversationRouter(s)
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	for i := 0; i < unansweredMessageBodies+3; i++ {
		require.Equal(t, 201, runtimeRequest(t, router, "POST", path, "", "", map[string]string{"body": fmt.Sprintf("message %02d", i)}).Code)
	}
	comments, err := s.Repo.ListComments(ctx, task, 1)
	require.NoError(t, err)
	a, err := s.Personas.GetAgentInstance(ctx, "chief")
	require.NoError(t, err)
	prompt, err := s.prompt(ctx, a, task, "", map[string]any{"comment_id": comments[0].ID})
	require.NoError(t, err)
	require.Contains(t, prompt, "2 older unanswered messages, read them with comments:")
	require.NotContains(t, prompt, ": message 00\n")
	require.Contains(t, prompt, ": message 02\n")
}
