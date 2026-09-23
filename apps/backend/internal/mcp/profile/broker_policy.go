package profile

// BrokerPolicyMetadataKey marks a task session launched on the orchestrator
// broker surface. Native launch, resume, prompt and steer restore that surface
// from the session, so a coordinator never regains ambient tools.
const BrokerPolicyMetadataKey = "orchestration_broker_policy"

// legacyBrokerPolicyMetadataKey marks broker sessions recorded under the
// retired assistant surface name. It selects the same broker surface.
const legacyBrokerPolicyMetadataKey = "assistant_broker_policy"

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
