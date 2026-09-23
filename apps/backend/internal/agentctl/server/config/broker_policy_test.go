package config

import (
	"strings"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBrokerPolicyReplacesAmbientTools(t *testing.T) {
	profile := mcpprofile.New(mcpprofile.SurfaceOrchestratorBroker, nil, nil)
	c := &Config{Defaults: InstanceDefaults{AutoApprovePermissions: true}, ShellEnabled: true}
	cfg := c.NewInstanceConfig(43210, &InstanceOverrides{McpProfile: &profile,
		McpServers: []McpServerConfig{{Name: "external", Command: "mutate"}},
		Env: []string{"KANDEV_API_URL=http://kandev", "KANDEV_API_KEY=synthetic-token", "KANDEV_PERSONAL_ASSISTANT_ENABLED=true",
			"CLAUDE_CODE_EXECUTABLE=mutate", "NODE_OPTIONS=--require=mutate", "PATH=/bin"}})
	require.True(t, cfg.BrokerRestricted())
	require.False(t, cfg.AutoApprovePermissions)
	require.False(t, cfg.ShellEnabled)
	require.True(t, cfg.DisableAskQuestion)
	require.Len(t, cfg.McpServers, 1)
	server := cfg.McpServers[0]
	require.Equal(t, BrokerMCPServerName, server.Name)
	require.Equal(t, []string{"kandev", BrokerMCPSubcommand}, server.Args)
	require.Equal(t, "synthetic-token", server.Env["KANDEV_API_KEY"])
	require.NotContains(t, server.Env, "KANDEV_PERSONAL_ASSISTANT_ENABLED")
	require.NotContains(t, server.Env, "PATH")
	require.NotContains(t, strings.Join(cfg.AgentEnv, "\n"), "mutate")
}

func TestLegacyAssistantSurfaceStaysBrokerRestricted(t *testing.T) {
	profile := mcpprofile.Context{Surface: "assistant-broker-v1"}
	c := &Config{ShellEnabled: true}
	cfg := c.NewInstanceConfig(43210, &InstanceOverrides{McpProfile: &profile})
	require.True(t, cfg.BrokerRestricted())
	require.False(t, cfg.ShellEnabled)
}

func TestOrdinarySurfaceKeepsItsTools(t *testing.T) {
	profile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil)
	c := &Config{ShellEnabled: true}
	cfg := c.NewInstanceConfig(43210, &InstanceOverrides{McpProfile: &profile,
		McpServers: []McpServerConfig{{Name: "external", Command: "tool"}}})
	require.False(t, cfg.BrokerRestricted())
	require.True(t, cfg.ShellEnabled)
	names := []string{}
	for _, server := range cfg.McpServers {
		names = append(names, server.Name)
	}
	require.Contains(t, names, "external")
	require.NotContains(t, names, BrokerMCPServerName)
}

func TestClaudeACPCommandVersionRequiresExactPin(t *testing.T) {
	require.Equal(t, "0.76.0", ClaudeACPCommandVersion([]string{"npx", "--yes", "--prefer-offline", "@agentclientprotocol/claude-agent-acp@0.76.0"}))
	require.Equal(t, "1.2.3-rc.1", ClaudeACPCommandVersion([]string{"@agentclientprotocol/claude-agent-acp@1.2.3-rc.1"}))
	for _, args := range [][]string{
		{"npx", "--yes", "@agentclientprotocol/claude-agent-acp"},
		{"npx", "--yes", "@agentclientprotocol/claude-agent-acp@latest"},
		{"npx", "--yes", "@agentclientprotocol/claude-agent-acp@^0.76.0"},
		{"npx", "--yes", "@other/claude-agent-acp@0.76.0"},
	} {
		require.Empty(t, ClaudeACPCommandVersion(args), args)
	}
}
