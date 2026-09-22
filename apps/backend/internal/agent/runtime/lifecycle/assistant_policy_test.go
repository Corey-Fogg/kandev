package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestAssistantReadOnlyLifecycleAdmission(t *testing.T) {
	profile := mcpprofile.New(mcpprofile.SurfaceAssistantBroker, nil, nil)
	req := LaunchRequest{ExecutorType: "local", McpProfile: &profile}
	require.NoError(t, validateAssistantLaunch(&req))
	for _, mutate := range []func(*LaunchRequest){
		func(r *LaunchRequest) { r.ExecutorType = "ssh" },
		func(r *LaunchRequest) { r.RepositoryID = "repo" },
		func(r *LaunchRequest) { r.SetupScript = "mutate" },
		func(r *LaunchRequest) { r.IsPassthrough = true },
	} {
		copy := req
		mutate(&copy)
		require.Error(t, validateAssistantLaunch(&copy))
	}
	info := &AgentProfileInfo{}
	require.NoError(t, validateAssistantCommand(&req, info, agents.NewClaudeACP(), agents.AssistantClaudeACPVersion(), nil, nil))
	require.Error(t, validateAssistantCommand(&req, info, agents.NewClaudeACP(), "0.0.0-unsupported", nil, nil))
	require.Error(t, validateAssistantCommand(&req, info, agents.NewClaudeACP(), agents.AssistantClaudeACPVersion(), []string{"--tools=Bash"}, nil))
	require.Error(t, validateAssistantCommand(&req, info, agents.NewClaudeACP(), agents.AssistantClaudeACPVersion(), nil, []string{"sh"}))
}

func TestAssistantLifecycleAcceptsAccountDirectoryAndEffort(t *testing.T) {
	profile := mcpprofile.New(mcpprofile.SurfaceAssistantBroker, nil, nil)
	req := LaunchRequest{ExecutorType: "local", McpProfile: &profile}
	version := agents.AssistantClaudeACPVersion()
	info := &AgentProfileInfo{
		ConfigOptions: map[string]string{"effort": "high"},
		EnvVars:       []settingsmodels.ProfileEnvVar{{Key: "CLAUDE_CONFIG_DIR", Value: "/home/example/.claude-personal"}},
	}
	require.NoError(t, validateAssistantCommand(&req, info, agents.NewClaudeACP(), version, nil, nil))
	info.ConfigOptions = map[string]string{"effort": "high", "permissions": "bypass"}
	require.Error(t, validateAssistantCommand(&req, info, agents.NewClaudeACP(), version, nil, nil))
	info.ConfigOptions = nil
	info.EnvVars = append(info.EnvVars, settingsmodels.ProfileEnvVar{Key: "NODE_OPTIONS", Value: "--require=/tmp/x"})
	require.Error(t, validateAssistantCommand(&req, info, agents.NewClaudeACP(), version, nil, nil))
}
