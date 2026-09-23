package backendapp

import (
	"context"
	"fmt"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCoordinatorPreparedSessionPersistsBrokerPolicy(t *testing.T) {
	for _, name := range []string{"bound", "revoked"} {
		t.Run(name, func(t *testing.T) {
			a, _, _, conversation := coordinatorConversationFixture(t)
			ctx := context.Background()
			require.NoError(t, a.taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{
				ID: "session", TaskID: conversation, State: taskmodels.TaskSessionStateCreated,
				Metadata: map[string]any{"native_conversation": true},
			}))
			launch := orchestrationruntime.Launch{TaskID: conversation, OnSessionPrepared: func(ctx context.Context, id string) error {
				if name == "revoked" {
					return fmt.Errorf("run superseded")
				}
				return a.taskRepo.SetSessionMetadataKey(ctx, id, "run_bound", true)
			}}
			prepared := orchestrationLaunchContext(&Repositories{Task: a.taskRepo}, launch)
			require.NotNil(t, prepared.McpProfile)
			require.Equal(t, mcpprofile.SurfaceOrchestratorBroker, prepared.McpProfile.Surface)
			err := prepared.OnSessionPrepared(ctx, "session")
			session, getErr := a.taskRepo.GetTaskSession(ctx, "session")
			require.NoError(t, getErr)
			require.Equal(t, true, session.Metadata["native_conversation"])
			// The policy is recorded before the run binds, so even a failed
			// binding leaves the session restricted.
			require.True(t, mcpprofile.SessionUsesBroker(session.Metadata))
			if name == "revoked" {
				require.ErrorContains(t, err, "run superseded")
				return
			}
			require.NoError(t, err)
			require.Equal(t, true, session.Metadata["run_bound"])
			require.NotContains(t, session.Metadata, "assistant_broker_policy")
		})
	}
}
