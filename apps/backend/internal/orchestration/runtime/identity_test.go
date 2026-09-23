package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	settings "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestration/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestPromptNamesTheOrchestratorAndItsSettings(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	s.Manager = &fakeTaskManager{}
	_, err := s.Repo.SetOrchestratorDisplayName(ctx, "chief", "Jeb")
	require.NoError(t, err)
	require.NoError(t, s.Repo.SaveOrchestratorSettings(ctx, "chief", models.OrchestratorSettings{AskBeforeCreate: true, AutoCommentSource: true}))
	persona, err := s.Personas.GetAgentInstance(ctx, "chief")
	require.NoError(t, err)
	require.Equal(t, "Jeb", persona.Name, "the instance name replaces the role name")
	prompt, err := s.prompt(ctx, persona, task, "", nil)
	require.NoError(t, err)
	require.Contains(t, prompt, "Your name in this workspace: Jeb\n")
	require.Contains(t, prompt, "Settings: ask before creating tasks=on; automatic source issue comment=on; automatic move of source issue to done on completion=off\n")

	_, err = s.Repo.SetOrchestratorDisplayName(ctx, "chief", "")
	require.NoError(t, err)
	persona, err = s.Personas.GetAgentInstance(ctx, "chief")
	require.NoError(t, err)
	require.Equal(t, "Chief of staff", persona.Name)
}

func TestStallIsRecordedOnTheTaskEvenWhenPaused(t *testing.T) {
	s, db, _ := newRuntime(t)
	ctx := context.Background()
	withTaskMetadata(s)
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	task := delegate(s, "delegated", v1.TaskStateInProgress)
	data := map[string]any{"task_id": "delegated", "session_id": "worker", "prompt_generation": 1, "stalled_for": 5 * time.Minute}
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.AgentStalled, "test", data)))
	require.Equal(t, &models.TaskStall{Outcome: stallNoProgress, StalledFor: "5m0s", SessionID: "worker", DetectedAt: now}, models.TaskStallFromMetadata(task.Metadata))
	require.Len(t, queuedCallbacks(t, db), 1, "recording the stall still wakes the coordinator")

	_, err := db.Exec(`UPDATE runs SET status='finished'`)
	require.NoError(t, err)
	_, err = s.Personas.UpdateAgentStatus(ctx, "chief", settings.AgentStatusPaused, "")
	require.NoError(t, err)
	delete(task.Metadata, models.MetaTaskStall)
	data["prompt_generation"] = 2
	_ = s.onEvent(ctx, bus.NewEvent(events.AgentStalled, "test", data))
	require.NotNil(t, models.TaskStallFromMetadata(task.Metadata), "a paused coordinator's task still shows the stall")
}

func TestHumanMetricsAreScopedAndNeverCreateAConversation(t *testing.T) {
	s, db, conversation := newRuntime(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedDelegatedOutcome(t, db, "done", "chief", "COMPLETED", now.Add(-time.Hour), now)
	seedUsage(t, db, "conversation-usage", conversation, "priced", 20000, now)
	result, err := s.CoordinatorMetrics(ctx, "ws", "chief", 7)
	require.NoError(t, err)
	require.Equal(t, 1, result.Completed)
	require.InDelta(t, 2.0, result.CostUSD, 0.001, "the conversation's cost is included")
	_, err = s.CoordinatorMetrics(ctx, "other", "chief", 7)
	require.ErrorIs(t, err, models.ErrProposalNotFound)
	_, err = s.CoordinatorMetrics(ctx, "ws", "chief", 14)
	require.Error(t, err)

	_, err = db.Exec(`DELETE FROM orchestration_conversations`)
	require.NoError(t, err)
	_, err = s.CoordinatorMetrics(ctx, "ws", "chief", 30)
	require.NoError(t, err)
	var conversations int
	require.NoError(t, db.Get(&conversations, `SELECT COUNT(*) FROM orchestration_conversations`))
	require.Zero(t, conversations, "reading metrics never creates the conversation")
}

func TestBrokerListsTheCallersProposals(t *testing.T) {
	s, _, conversation := newRuntime(t)
	s.Manager = &fakeTaskManager{}
	askBeforeCreate(t, s)
	router, token, run := workspaceControlCaller(t, s, conversation)
	require.Equal(t, 202, runtimeRequest(t, router, "POST", tasksPath, token, run, map[string]any{"title": "Fix login"}).Code)
	response := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/proposals?status=pending", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var page struct {
		Entries    []models.TaskProposal `json:"entries"`
		NextCursor string                `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Len(t, page.Entries, 1)
	require.Equal(t, "Fix login", page.Entries[0].Spec.Title)
	require.Equal(t, 422, runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/proposals?status=bogus", token, run, nil).Code)

	capabilities := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/capabilities?limit=100", token, run, nil)
	require.Equal(t, 200, capabilities.Code)
	require.Contains(t, capabilities.Body.String(), `"name":"task_proposals","description":"List your task proposals awaiting or after the user's decision, newest first.","effect":"read"`)
}
