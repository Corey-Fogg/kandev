package runtime

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// guardedStart binds the session and then applies the coordinator dispatch
// guard, as the native launch path does.
func guardedStart(s *Service, task string, sessions *[]string) func(context.Context, Launch) error {
	return func(ctx context.Context, l Launch) error {
		id := "session-" + string(rune('a'+len(*sessions)))
		*sessions = append(*sessions, id)
		if err := l.OnSessionPrepared(ctx, id); err != nil {
			return err
		}
		session := &taskmodels.TaskSession{ID: id, TaskID: task,
			Metadata: map[string]any{mcpprofile.BrokerPolicyMetadataKey: string(mcpprofile.SurfaceOrchestratorBroker)}}
		return s.CheckCoordinatorSession(ctx, task, session)
	}
}

func TestCallbackQueuedBeforeUserMessageLaunchesAtCurrentIntent(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	delegate(s, "delegated", v1.TaskStateReview)
	require.NoError(t, s.taskCallback(ctx, "delegated"))
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	require.Equal(t, 201, runtimeRequest(t, conversationRouter(s), "POST", path, "", "", map[string]string{"body": "Also check the docs"}).Code)

	var sessions []string
	s.Start = guardedStart(s, task, &sessions)
	callback, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.Equal(t, callbackReason, callback.Reason, "the older callback is claimed first")
	handled, err := s.Process(ctx, callback)
	require.True(t, handled)
	require.NoError(t, err, "a callback queued before a user message launches at the current intent")
	stored, err := s.Runs.GetRunByID(ctx, callback.ID)
	require.NoError(t, err)
	require.EqualValues(t, statusClaimed, stored.Status)
	_, err = s.Runs.FinishRun(ctx, callback.ID, "finished", nil)
	require.NoError(t, err)

	comment, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.Equal(t, commentReason, comment.Reason)
	handled, err = s.Process(ctx, comment)
	require.True(t, handled)
	require.NoError(t, err, "the user message still launches after the callback")
	require.Len(t, sessions, 2)
}

func TestRetriedCallbackWithStaleIntentLaunches(t *testing.T) {
	s, db, task := newRuntime(t)
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, "chief", task, callbackReason, "retry:old", map[string]any{intentRevisionKey: 1}))
	_, err := db.Exec(`INSERT INTO orchestration_conversation_intents(task_id,revision) VALUES(?,3)
		ON CONFLICT(task_id) DO UPDATE SET revision=3`, task)
	require.NoError(t, err)
	var sessions []string
	s.Start = guardedStart(s, task, &sessions)
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	handled, err := s.Process(ctx, run)
	require.True(t, handled)
	require.NoError(t, err, "a retried callback takes the intent current at launch")
}
