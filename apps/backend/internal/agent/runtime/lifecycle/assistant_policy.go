package lifecycle

import (
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	assistantPolicyMetadata = "assistant_broker_policy"
	assistantClaudeAgent    = "claude-acp"
)

func assistantRestrictedLaunch(req *LaunchRequest) bool {
	return req.McpProfile != nil && req.McpProfile.Surface == mcpprofile.SurfaceAssistantBroker
}

func validateAssistantLaunch(req *LaunchRequest) error {
	if !assistantRestrictedLaunch(req) {
		return nil
	}
	if req.ExecutorType != string(models.ExecutorTypeLocal) || req.IsPassthrough || req.RepositoryID != "" || req.RepositoryPath != "" ||
		req.SetupScript != "" || req.CopyFiles != "" || req.UseWorktree || len(req.WorkspaceFolders) != 0 {
		return fmt.Errorf("assistant policy requires a repository-free local executor without preparation scripts")
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	req.Metadata[assistantPolicyMetadata] = string(mcpprofile.SurfaceAssistantBroker)
	return nil
}

func validateAssistantCommand(req *LaunchRequest, profile *AgentProfileInfo, agent agents.Agent, version string, flags, prefix []string) error {
	if !assistantRestrictedLaunch(req) {
		return nil
	}
	if agent.ID() != assistantClaudeAgent || version != agents.AssistantClaudeACPVersion() || len(flags) != 0 || len(prefix) != 0 || profile == nil ||
		profile.CLIPassthrough || !assistantProfileEnvSupported(profile) || !assistantConfigOptionsSupported(profile.ConfigOptions) {
		return fmt.Errorf("assistant policy unsupported by the selected runtime")
	}
	return nil
}

func assistantProfileEnvSupported(profile *AgentProfileInfo) bool {
	for _, env := range profile.EnvVars {
		if !agents.AssistantProfileEnvAllowed(env.Key, env.Value, env.SecretID) {
			return false
		}
	}
	return true
}

// Effort is the only provider option admission accepts; it changes reasoning
// depth, not the tool surface.
func assistantConfigOptionsSupported(options map[string]string) bool {
	for option := range options {
		if option != "effort" {
			return false
		}
	}
	return true
}
