package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessSettlesRegisteredRunWithoutLaunchWhenOrchestrationDisabled(t *testing.T) {
	svc, _, _ := newRuntime(t)
	ctx := context.Background()
	_, err := svc.Send(ctx, "ws", "chief", "disabled-delivery", "Daily PR review")
	require.NoError(t, err)
	run, err := svc.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NotNil(t, run)

	svc.AssistantEnabled = false
	svc.Start = func(context.Context, Launch) error {
		t.Fatal("a disabled orchestration runtime must not launch")
		return nil
	}
	handled, err := svc.Process(ctx, run)
	require.ErrorIs(t, err, errOrchestrationDisabled)
	require.True(t, handled, "the run is settled here, not handed to another runtime")
	settled, err := svc.Runs.GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, statusFailed, string(settled.Status))
}
