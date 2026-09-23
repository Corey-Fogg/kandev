package config

import (
	"os"
	"regexp"
	"strings"
)

// BrokerToolPolicy is the Claude ACP session policy for a broker-restricted
// coordinator: no built-in tools, only the managed broker MCP server.
const BrokerToolPolicy = "claude-broker-v1"

// BrokerMCPServerName names the single MCP server a broker-restricted agent
// receives. Its tools surface as mcp__kandev_orchestrator__<tool>.
const BrokerMCPServerName = "kandev_orchestrator"

// BrokerMCPSubcommand is the agentctl subcommand that serves the broker.
const BrokerMCPSubcommand = "orchestrator-mcp"

const claudeACPPackage = "@agentclientprotocol/claude-agent-acp"

var exactPackageVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// ClaudeACPCommandVersion returns the exact Claude ACP version a launch command
// pins, or "" when the command does not pin one. The ACP handshake must then
// report exactly this version before the broker session policy is sent.
func ClaudeACPCommandVersion(args []string) string {
	for _, arg := range args {
		version, ok := strings.CutPrefix(arg, claudeACPPackage+"@")
		if ok && exactPackageVersion.MatchString(version) {
			return version
		}
	}
	return ""
}

// BrokerRestricted reports whether the instance runs a coordinator on the
// broker surface: no shell and no MCP server other than the managed broker.
func (c *InstanceConfig) BrokerRestricted() bool {
	return c.McpProfile != nil && c.McpProfile.IsBroker()
}

// applyBrokerPolicy replaces every ambient MCP attachment. The executable
// is this managed agentctl binary, never a profile-supplied command.
func applyBrokerPolicy(c *InstanceConfig) {
	if !c.BrokerRestricted() {
		return
	}
	c.AutoApprovePermissions = false
	c.ShellEnabled = false
	c.DisableAskQuestion = true
	executable, err := os.Executable()
	if err != nil {
		c.McpServers = nil
		return
	}
	brokerEnv := map[string]string{}
	kept := []string{}
	for _, entry := range c.AgentEnv {
		key, value, _ := strings.Cut(entry, "=")
		switch key {
		case "CLAUDE_CODE_EXECUTABLE", "NODE_OPTIONS", "NODE_PATH":
			continue
		case "KANDEV_API_URL", "KANDEV_API_KEY", "KANDEV_RUN_ID", "KANDEV_TASK_ID", "KANDEV_AGENT_ID", "KANDEV_WORKSPACE_ID", "KANDEV_RUNTIME_API_PREFIX":
			brokerEnv[key] = value
		}
		kept = append(kept, entry)
	}
	c.AgentEnv = kept
	c.McpServers = []McpServerConfig{{Name: BrokerMCPServerName, Type: "stdio", Command: executable,
		Args: []string{"kandev", BrokerMCPSubcommand}, Env: brokerEnv}}
}

// RefreshBrokerPolicy reapplies the immutable restriction after runtime
// credentials are refreshed or a managed process is configured for restart.
func RefreshBrokerPolicy(c *InstanceConfig) { applyBrokerPolicy(c) }
