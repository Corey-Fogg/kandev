package process

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

func brokerCoordinatorManager(t *testing.T, agentType string) *Manager {
	profile := mcpprofile.New(mcpprofile.SurfaceOrchestratorBroker, nil, nil)
	return &Manager{cfg: &config.InstanceConfig{AgentArgs: []string{"cat"}, AgentType: agentType, WorkDir: t.TempDir(),
		Protocol: agent.ProtocolACP, McpProfile: &profile, ShellEnabled: true, AutoApprovePermissions: true,
		McpServers: []config.McpServerConfig{{Name: "ambient", Command: "tool"}},
		AgentEnv:   []string{"CLAUDE_CONFIG_DIR=/synthetic/account"}}, logger: newTestLogger(t)}
}

// A provider that can run a coordinator gets the broker restriction; only
// Claude also receives the provider session policy.
func TestBrokerCoordinatorExposesOnlyTheBroker(t *testing.T) {
	for _, agentType := range []string{"claude-acp", "mock-agent"} {
		t.Run(agentType, func(t *testing.T) {
			m := brokerCoordinatorManager(t, agentType)
			require.NoError(t, m.buildAdapterConfig())
			t.Cleanup(func() { _ = m.adapter.Close() })
			require.True(t, m.adapterCfg.BrokerRestricted)
			require.False(t, m.adapterCfg.AutoApprove)
			require.False(t, m.cfg.ShellEnabled)
			require.Len(t, m.adapterCfg.McpServers, 1)
			require.Equal(t, config.BrokerMCPServerName, m.adapterCfg.McpServers[0].Name)
			require.Equal(t, []string{"kandev", config.BrokerMCPSubcommand}, m.adapterCfg.McpServers[0].Args)
			require.Contains(t, m.cfg.AgentEnv, "CLAUDE_CONFIG_DIR=/synthetic/account")
			if agentType == "claude-acp" {
				require.Equal(t, config.BrokerToolPolicy, m.adapterCfg.ToolPolicy)
			} else {
				require.Empty(t, m.adapterCfg.ToolPolicy)
			}
		})
	}
}

// A provider whose built-in shell and file tools cannot be switched off never
// starts a broker-only coordinator.
func TestBrokerCoordinatorRefusesProvidersWithBuiltInTools(t *testing.T) {
	for _, agentType := range []string{"codex-acp", "gemini", "copilot-acp", "opencode-acp", "auggie", "cursor-acp"} {
		t.Run(agentType, func(t *testing.T) {
			m := brokerCoordinatorManager(t, agentType)
			require.ErrorContains(t, m.buildAdapterConfig(), "cannot run a broker-only coordinator")
			require.Nil(t, m.adapter)
		})
	}
}

func TestOrdinarySessionIsNotBrokerRestricted(t *testing.T) {
	profile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil)
	m := &Manager{cfg: &config.InstanceConfig{AgentArgs: []string{"cat"}, AgentType: "claude-acp", WorkDir: t.TempDir(),
		Protocol: agent.ProtocolACP, McpProfile: &profile}, logger: newTestLogger(t)}
	require.NoError(t, m.buildAdapterConfig())
	t.Cleanup(func() { _ = m.adapter.Close() })
	require.False(t, m.adapterCfg.BrokerRestricted)
	require.Empty(t, m.adapterCfg.ToolPolicy)
}
