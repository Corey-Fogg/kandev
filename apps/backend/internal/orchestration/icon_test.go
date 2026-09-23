package orchestration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
)

func orchestratorIcon(a *models.AgentInstance) string {
	var settings struct {
		Icon string `json:"orchestrator_icon"`
	}
	_ = json.Unmarshal([]byte(a.Settings), &settings)
	return settings.Icon
}

func TestPersonaIconPreservesConfiguration(t *testing.T) {
	a := &models.AgentInstance{Settings: `{"delegation_context":"Work profile"}`}
	if err := setOrchestratorIcon(a, "🧭"); err != nil {
		t.Fatal(err)
	}
	if orchestratorIcon(a) != "🧭" || !strings.Contains(a.Settings, "Work profile") {
		t.Fatal(a.Settings)
	}
	previous := a.Settings
	if err := setOrchestratorIcon(a, "invalid"); err == nil || a.Settings != previous {
		t.Fatal("invalid icon changed settings")
	}
	if err := setOrchestratorIcon(a, ""); err != nil || orchestratorIcon(a) != "" {
		t.Fatal("could not restore initials")
	}
}
