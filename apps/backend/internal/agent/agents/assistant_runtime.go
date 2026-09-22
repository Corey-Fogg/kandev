package agents

import "path/filepath"

// AssistantClaudeACPPackage is the only provider package the restricted
// assistant broker launches.
const AssistantClaudeACPPackage = claudeACPPackage

// AssistantClaudeACPEnvConfigDir selects the Claude account directory. It is
// the only profile environment variable a restricted launch accepts.
const AssistantClaudeACPEnvConfigDir = "CLAUDE_CONFIG_DIR"

// AssistantClaudeACPVersion returns the Claude ACP version the restricted
// broker accepts: the release's reviewed managed default. Tying the policy to
// the catalogue keeps admission, launch and handshake checks in step when a
// release moves the default, instead of rejecting every turn after an upgrade.
func AssistantClaudeACPVersion() string {
	return MustDefaultManagedNPMRuntimeVersion(claudeACPPackage)
}

// AssistantProfileEnvAllowed reports whether a profile environment variable
// may accompany a restricted assistant launch. Only a literal absolute Claude
// account directory is accepted. The broker disables setting sources, hooks,
// plugins and ambient MCP servers, so the directory contributes the account
// login and nothing that widens the tool surface.
func AssistantProfileEnvAllowed(key, value, secretID string) bool {
	return key == AssistantClaudeACPEnvConfigDir && secretID == "" && filepath.IsAbs(value) && filepath.Clean(value) == value
}
