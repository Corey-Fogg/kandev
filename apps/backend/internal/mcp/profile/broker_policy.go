package profile

// BrokerPolicyMetadataKey marks a task session launched on the orchestrator
// broker surface. Native launch, resume, prompt and steer restore that surface
// from the session, so a coordinator never regains ambient tools.
const BrokerPolicyMetadataKey = "orchestration_broker_policy"

// legacyBrokerPolicyMetadataKey marks broker sessions recorded under the
// retired assistant surface name. It selects the same broker surface.
const legacyBrokerPolicyMetadataKey = "assistant_broker_policy"

// IsBroker reports whether the profile selects the broker surface, including
// a stored profile that still names the retired assistant surface.
func (c Context) IsBroker() bool {
	return c.Surface == SurfaceOrchestratorBroker || c.Surface == legacyAssistantBroker
}

// SessionUsesBroker reports whether session metadata pins the broker surface.
// Any non-empty policy value counts, so an unrecognised value fails closed.
func SessionUsesBroker(metadata map[string]any) bool {
	for _, key := range []string{BrokerPolicyMetadataKey, legacyBrokerPolicyMetadataKey} {
		if value, _ := metadata[key].(string); value != "" {
			return true
		}
	}
	return false
}

// BrokerCapableAgent reports whether an agent type can run as a broker-only
// coordinator. claude-acp receives a session policy that switches its
// built-in tools off, and mock-agent has none. Every other provider keeps
// built-in shell and file tools that the broker restriction cannot remove.
func BrokerCapableAgent(agentType string) bool {
	return agentType == "claude-acp" || agentType == "mock-agent"
}
