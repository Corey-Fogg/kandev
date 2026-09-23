package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// Claude ACP reports an OAuth refresh collision as an assistant diagnostic
// chunk and then fails session/prompt with the same text (the terminal copy is
// sanitized, so "token: another" reads "token: ***"). Only the claude-acp
// rules recognize it; the chunk must still count as a diagnostic, not output,
// or a coordinator turn that never started is refused its automatic retry.
const (
	refreshContentionChunk    = "Failed to refresh OAuth token: another Claude Code process is refreshing it or exited mid-refresh. This is usually transient; retry in a minute, and if it persists close other Claude Code processes or sign in again"
	refreshContentionTerminal = "Internal error: Failed to refresh OAuth token: *** Claude Code process is refreshing it or exited mid-refresh. This is usually transient; retry in a minute, and if it persists close other Claude Code processes or sign in again"
)

func refreshContentionFailure() watcher.AgentEventData {
	return watcher.AgentEventData{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		AgentID:          "claude-acp",
		PromptGeneration: 1,
		ErrorMessage:     refreshContentionTerminal,
	}
}

func TestRefreshContentionStreamDiagnosticIsNotOutput(t *testing.T) {
	var service Service
	service.beginPromptAttempt("session-1", "execution-1", 1, false)
	service.observeProviderDiagnosticFrom("session-1", "execution-1", 1, "claude-acp", refreshContentionChunk)

	got := service.withPromptAttemptEvidence(refreshContentionFailure())
	if !got.EvidenceKnown || got.OutputObserved || got.EffectObserved {
		t.Fatalf("evidence known=%v output=%v effect=%v, want known and no output", got.EvidenceKnown, got.OutputObserved, got.EffectObserved)
	}
	if !service.promptAttemptPreResultSafe(got) {
		t.Fatal("refresh contention before any output was not pre-result safe")
	}
}

func TestRefreshContentionLifecycleDiagnosticIsNotOutput(t *testing.T) {
	var service Service
	service.beginPromptAttempt("session-1", "execution-1", 1, false)
	data := refreshContentionFailure()
	data.EvidenceKnown = true
	data.ProviderDiagnosticCandidate = true
	data.ProviderDiagnosticText = refreshContentionChunk

	got := service.withPromptAttemptEvidence(data)
	if !got.EvidenceKnown || got.OutputObserved {
		t.Fatalf("evidence known=%v output=%v, want known and no output", got.EvidenceKnown, got.OutputObserved)
	}
}

func TestRefreshContentionDiagnosticFollowedByProseIsOutput(t *testing.T) {
	var service Service
	service.beginPromptAttempt("session-1", "execution-1", 1, false)
	service.observeProviderDiagnosticFrom("session-1", "execution-1", 1, "claude-acp", refreshContentionChunk)
	service.observePromptAttempt("session-1", "execution-1", 1, true, false)

	if got := service.withPromptAttemptEvidence(refreshContentionFailure()); !got.OutputObserved {
		t.Fatal("real output after the diagnostic was not recorded")
	}
}
