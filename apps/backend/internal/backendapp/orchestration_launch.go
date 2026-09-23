package backendapp

import (
	"context"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	orchexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
)

// orchestrationLaunchContext launches every coordinator conversation on the
// backend-owned broker surface, so the agent never falls back to an ambient
// CLI, API key or tool set.
func orchestrationLaunchContext(repos *Repositories, launch orchestrationruntime.Launch) orchexecutor.LaunchContext {
	value := mcpprofile.New(mcpprofile.SurfaceOrchestratorBroker, nil, nil)
	prepared := func(ctx context.Context, sessionID string) error {
		// The policy is recorded before runtime credentials are bound, so every
		// native resume, prompt or steer of this session restores the broker.
		if err := repos.Task.SetSessionMetadataKey(ctx, sessionID, mcpprofile.BrokerPolicyMetadataKey, string(mcpprofile.SurfaceOrchestratorBroker)); err != nil {
			return err
		}
		if launch.OnSessionPrepared == nil {
			return nil
		}
		return launch.OnSessionPrepared(ctx, sessionID)
	}
	return orchexecutor.LaunchContext{McpProfile: &value, ExecutorProfileID: launch.ExecutorID, Prompt: launch.Prompt, Env: launch.Env, OnSessionPrepared: prepared}
}
