package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func runtimeNotes(t *testing.T, s *Service, task string) []*models.TaskComment {
	t.Helper()
	rows, err := s.Repo.ListComments(context.Background(), task, 20)
	require.NoError(t, err)
	notes := []*models.TaskComment{}
	for _, row := range rows {
		if row.Source == turnFailureSource {
			notes = append(notes, row)
		}
	}
	return notes
}

func TestWatchdogFailsTurnThatNeverBoundASession(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, "chief", task, commentReason, "stuck", nil))
	stuck, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()

	require.NoError(t, s.FailUnboundRuns(ctx, now))
	current, err := s.Runs.GetRunByID(ctx, stuck.ID)
	require.NoError(t, err)
	require.EqualValues(t, statusClaimed, current.Status, "a recent launch keeps its claim")

	later := now.Add(unboundRunTimeout + time.Second)
	require.NoError(t, s.FailUnboundRuns(ctx, later))
	require.NoError(t, s.FailUnboundRuns(ctx, later))
	current, err = s.Runs.GetRunByID(ctx, stuck.ID)
	require.NoError(t, err)
	require.EqualValues(t, statusFailed, current.Status)
	require.Contains(t, current.ErrorMessage, "did not start")
	require.Len(t, runtimeNotes(t, s, task), 1, "the conversation hears about the failure once")

	// A launch that binds its session after the watchdog cannot act.
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, stuck.ID, workspaceCoordinatorAudience, stuck.Payload, "late"))
	late := &taskmodels.TaskSession{ID: "late", TaskID: task, Metadata: map[string]any{mcpprofile.BrokerPolicyMetadataKey: string(mcpprofile.SurfaceOrchestratorBroker)}}
	require.ErrorIs(t, s.CheckCoordinatorSession(ctx, task, late), models.ErrConflict)

	// The failed turn is retried by run id; a bound retry keeps its claim.
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/orchestration"), &Handler{Service: s})
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/tasks/"+task+"/retry", "", "", map[string]string{"run_id": stuck.ID, "action": "resume"})
	require.Equal(t, 202, response.Code, response.Body.String())
	retried, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, retried.ID, workspaceCoordinatorAudience, retried.Payload, "session"))
	require.NoError(t, s.FailUnboundRuns(ctx, later.Add(time.Hour)))
	current, err = s.Runs.GetRunByID(ctx, retried.ID)
	require.NoError(t, err)
	require.EqualValues(t, statusClaimed, current.Status)
}

func TestLaunchFailurePostsConversationNote(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	s.Start = func(context.Context, Launch) error { return errors.New("executor profile unavailable") }
	require.NoError(t, s.QueueTurn(ctx, "chief", task, commentReason, "launch", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	handled, err := s.Process(ctx, run)
	require.True(t, handled)
	require.Error(t, err)
	notes := runtimeNotes(t, s, task)
	require.Len(t, notes, 1)
	require.Contains(t, notes[0].Body, "executor profile unavailable")
	require.Equal(t, "chief", notes[0].AuthorID)
}
