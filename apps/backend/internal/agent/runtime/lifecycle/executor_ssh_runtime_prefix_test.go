package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSHRemoteAgentEnvForwardsRuntimeAPIPrefix(t *testing.T) {
	env := sshRemoteAgentEnv(&ExecutorCreateRequest{Env: map[string]string{
		"KANDEV_RUN_TOKEN":          "run-token",
		"KANDEV_RUNTIME_API_PREFIX": "/api/v1/orchestration",
		"UNRELATED_HOST_SECRET":     "must-not-forward",
	}})
	require.Equal(t, "/api/v1/orchestration", env["KANDEV_RUNTIME_API_PREFIX"])
	require.Equal(t, "run-token", env["KANDEV_RUN_TOKEN"])
	require.NotContains(t, env, "UNRELATED_HOST_SECRET")
}
