package acp

import (
	"context"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestClaudeBrokerAdapterNewAndResume(t *testing.T) {
	a, capture := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	a.cfg.ToolPolicy = "claude-broker-v1"
	a.cfg.ToolPolicyVersion = "0.76.0"
	a.agentID = "claude-acp"
	a.agentInfo = &AgentInfo{Name: "@agentclientprotocol/claude-agent-acp", Version: "0.76.0"}
	servers := []types.McpServer{{Name: "kandev_orchestrator", Command: "/owned/agentctl", Args: []string{"kandev", "orchestrator-mcp"}}, {Name: "untrusted", Command: "mutate"}}
	_, err := a.NewSession(context.Background(), servers)
	require.ErrorContains(t, err, "attachment")
	_, err = a.NewSession(context.Background(), servers[:1])
	require.NoError(t, err)
	require.Len(t, capture.newRequest.McpServers, 1)
	options := capture.newRequest.Meta["claudeCode"].(map[string]any)["options"].(map[string]any)
	require.Empty(t, options["tools"])
	require.Empty(t, options["settingSources"])
	require.Equal(t, true, options["strictMcpConfig"])
	require.Equal(t, map[string]any{"disableAllHooks": true, "permissions": map[string]any{"allow": []any{"mcp__kandev_orchestrator__*"}}}, options["settings"])
	require.NotContains(t, options, "allowedTools", "a bare allowedTools entry would shadow canUseTool")
	require.Equal(t, map[string]any{"disable-slash-commands": nil, "permission-mode": "dontAsk"}, options["extraArgs"])
	require.NoError(t, a.LoadSession(context.Background(), "previous-restricted-session", servers[:1]))
	require.Equal(t, capture.newRequest.Meta, capture.loadRequest.Meta)
	require.Error(t, a.SetMode(context.Background(), "bypassPermissions"))
	require.Error(t, a.SetConfigOption(context.Background(), "mode", "bypassPermissions"))
	response, err := a.handlePermissionRequest(context.Background(), &PermissionRequest{})
	require.NoError(t, err)
	require.True(t, response.Cancelled)
}

func TestClaudeBrokerAdapterRejectsUnprovenVersion(t *testing.T) {
	a, _ := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	a.cfg.ToolPolicy = "claude-broker-v1"
	a.agentID = "claude-acp"
	a.agentInfo = &AgentInfo{Version: "0.75.2"}
	_, err := a.NewSession(context.Background(), nil)
	require.ErrorContains(t, err, "unsupported")
}

func TestClaudeBrokerAdapterRequiresHandshakeToMatchLaunchedVersion(t *testing.T) {
	servers := []types.McpServer{{Name: "kandev_orchestrator", Command: "/owned/agentctl", Args: []string{"kandev", "orchestrator-mcp"}}}
	for _, row := range []struct {
		launched, reported string
		ok                 bool
	}{
		{"0.76.0", "0.76.0", true},
		{"0.76.0", "0.75.1", false},
		{"", "0.76.0", false},
	} {
		a, _ := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
		a.cfg.ToolPolicy = "claude-broker-v1"
		a.cfg.ToolPolicyVersion = row.launched
		a.agentID = "claude-acp"
		a.agentInfo = &AgentInfo{Name: "@agentclientprotocol/claude-agent-acp", Version: row.reported}
		_, err := a.NewSession(context.Background(), servers)
		if row.ok {
			require.NoError(t, err, "%+v", row)
		} else {
			require.ErrorContains(t, err, "unsupported", "%+v", row)
		}
	}
}

func TestBrokerSessionAcceptsProfileModeAndEffortOnly(t *testing.T) {
	a, _ := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	a.cfg.BrokerRestricted = true
	for _, mode := range []string{"default", "auto"} {
		require.NoError(t, a.SetMode(context.Background(), mode))
	}
	for _, mode := range []string{"acceptEdits", "bypassPermissions", "plan"} {
		require.ErrorContains(t, a.SetMode(context.Background(), mode), "forbids")
	}
	for _, option := range []string{"fast", "permissions"} {
		require.ErrorContains(t, a.SetConfigOption(context.Background(), option, "on"), "forbids")
	}
	require.NotContains(t, errString(a.SetConfigOption(context.Background(), "effort", "high")), "forbids")
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestNonClaudeBrokerSessionHasNoHostCapabilitiesOrProviderPolicy(t *testing.T) {
	a, capture := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	a.cfg.BrokerRestricted = true
	a.agentID = "codex-acp"
	require.Equal(t, acpsdk.ClientCapabilities{}, a.clientCapabilities())
	servers := []types.McpServer{{Name: "kandev_orchestrator", Command: "/owned/agentctl", Args: []string{"kandev", "orchestrator-mcp"}}}
	_, err := a.NewSession(context.Background(), servers)
	require.NoError(t, err)
	require.Nil(t, capture.newRequest.Meta)
	require.Len(t, capture.newRequest.McpServers, 1)
	require.ErrorContains(t, a.SetMode(context.Background(), "full-access"), "forbids")
	require.ErrorContains(t, a.SetConfigOption(context.Background(), "approval", "never"), "forbids")
}
