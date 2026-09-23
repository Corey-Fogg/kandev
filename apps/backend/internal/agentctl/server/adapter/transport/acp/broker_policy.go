package acp

import (
	"fmt"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
)

// brokerRestricted reports whether this adapter serves a broker-only
// coordinator: no ACP host operations, mode changes or tool configuration.
func (a *Adapter) brokerRestricted() bool {
	return a.cfg.BrokerRestricted || a.cfg.ToolPolicy != ""
}

// brokerSessionMeta returns the provider session policy for a broker-only
// coordinator. Claude receives no built-in tools, no settings sources or
// plugins, and a permission rule that preapproves only the broker server.
// The broker authorizes every call server-side, so the rule lives in the
// session settings rather than in allowedTools, which would shadow the
// canUseTool callback for those tools.
func (a *Adapter) brokerSessionMeta(servers []types.McpServer) (map[string]any, error) {
	if a.cfg.ToolPolicy == "" {
		return nil, nil
	}
	if a.cfg.ToolPolicy != config.BrokerToolPolicy || a.agentID != "claude-acp" || a.agentInfo == nil ||
		a.agentInfo.Name != "@agentclientprotocol/claude-agent-acp" || a.cfg.ToolPolicyVersion == "" || a.agentInfo.Version != a.cfg.ToolPolicyVersion {
		return nil, fmt.Errorf("broker policy unsupported by this provider version")
	}
	if len(servers) != 1 || servers[0].Name != config.BrokerMCPServerName || servers[0].Command == "" || servers[0].URL != "" {
		return nil, fmt.Errorf("broker attachment policy requires only the managed broker")
	}
	return map[string]any{
		"disableBuiltInTools": true,
		"claudeCode": map[string]any{"options": map[string]any{
			"tools": []string{}, "settingSources": []string{}, "plugins": []any{},
			"strictMcpConfig": true,
			"settings": map[string]any{
				"disableAllHooks": true,
				"permissions":     map[string]any{"allow": []string{"mcp__" + config.BrokerMCPServerName + "__*"}},
			},
			"enableFileCheckpointing": false,
			"extraArgs":               map[string]any{"disable-slash-commands": nil, "permission-mode": "dontAsk"},
		}},
	}, nil
}

func (a *Adapter) clientCapabilities() acpsdk.ClientCapabilities {
	if a.brokerRestricted() {
		return acpsdk.ClientCapabilities{}
	}
	return clientCapabilitiesForAgent(a.agentID, false)
}
