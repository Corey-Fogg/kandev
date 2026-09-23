package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestCoordinatorRemembersAndForgetsWorkspaceMemory(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	router, token, run := workspaceControlCaller(t, s, task)
	path := "/api/v1/orchestration/runtime/memory"

	saved := runtimeRequest(t, router, "POST", path, token, run, map[string]string{"key": "branch-policy", "content": "Branch from develop."})
	require.Equal(t, 200, saved.Code, saved.Body.String())
	replaced := runtimeRequest(t, router, "POST", path, token, run, map[string]string{"key": "branch-policy", "content": "Branch from main."})
	require.Equal(t, 200, replaced.Code, replaced.Body.String())
	var row models.AgentMemory
	require.NoError(t, json.Unmarshal(replaced.Body.Bytes(), &row))
	require.Equal(t, "Branch from main.", row.Content)

	for _, invalid := range []map[string]string{{"key": "", "content": "x"}, {"key": "empty", "content": " "}, {"key": "long", "content": strings.Repeat("x", maxMemoryContentBytes+1)}} {
		require.Equal(t, 422, runtimeRequest(t, router, "POST", path, token, run, invalid).Code)
	}
	require.NoError(t, s.Repo.UpsertAgentMemory(ctx, &models.AgentMemory{AgentProfileID: "other", Layer: coordinatorMemoryLayer, Key: "foreign", Content: "Other workspace", Metadata: "{}"}))
	foreign, err := s.Repo.GetAgentMemory(ctx, "other", coordinatorMemoryLayer, "foreign")
	require.NoError(t, err)
	require.Equal(t, 404, runtimeRequest(t, router, "DELETE", path+"/"+foreign.ID, token, run, nil).Code, "another coordinator's memory is out of scope")

	listed := runtimeRequest(t, router, "GET", path, token, run, nil)
	require.Equal(t, 200, listed.Code)
	require.Contains(t, listed.Body.String(), "Branch from main.")
	require.NotContains(t, listed.Body.String(), "Other workspace")

	a, err := s.Personas.GetAgentInstance(ctx, "chief")
	require.NoError(t, err)
	prompt, err := s.prompt(ctx, a, task, map[string]any{})
	require.NoError(t, err)
	require.Contains(t, prompt, "branch-policy (id="+row.ID+"): Branch from main.")

	require.Equal(t, 200, runtimeRequest(t, router, "DELETE", path+"/"+row.ID, token, run, nil).Code)
	rows, err := s.Repo.ListAgentMemory(ctx, "chief")
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestCoordinatorMemoryIsBounded(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	for i := 0; i < maxCoordinatorMemories; i++ {
		require.NoError(t, s.Repo.UpsertAgentMemory(ctx, &models.AgentMemory{AgentProfileID: "chief", Layer: coordinatorMemoryLayer, Key: string(rune('a'+i%26)) + strings.Repeat("k", i), Content: "entry", Metadata: "{}"}))
	}
	router, token, run := workspaceControlCaller(t, s, task)
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/memory", token, run, map[string]string{"key": "one-more", "content": "Rejected"})
	require.Equal(t, 422, response.Code, response.Body.String())
}

func TestPromptMemoryKeepsNewestEntriesWithinBudget(t *testing.T) {
	now := time.Now()
	rows := []*models.AgentMemory{}
	for i := 0; i < 10; i++ {
		rows = append(rows, &models.AgentMemory{ID: string(rune('a' + i)), Key: "k", Content: strings.Repeat("x", maxMemoryContentBytes), UpdatedAt: now.Add(time.Duration(i) * time.Minute)})
	}
	selected, omitted := promptMemory(rows)
	require.NotEmpty(t, selected)
	require.Equal(t, len(rows), len(selected)+omitted)
	require.Equal(t, "j", selected[0].ID, "the most recently updated entry comes first")
	used := 0
	for _, row := range selected {
		used += len(row.Key) + len(row.Content)
	}
	require.LessOrEqual(t, used, promptMemoryBudgetBytes)
}

func TestMemoryReadsIgnoreRetainedLegacyColumns(t *testing.T) {
	s, db, _ := newRuntime(t)
	ctx := context.Background()
	for _, column := range []string{"owner_user_id TEXT NOT NULL DEFAULT ''", "scope TEXT NOT NULL DEFAULT 'workspace'", "confirmed INTEGER NOT NULL DEFAULT 0", "forgotten_at TIMESTAMP"} {
		_, err := db.Exec("ALTER TABLE orchestration_memory ADD COLUMN " + column)
		require.NoError(t, err)
	}
	require.NoError(t, s.Repo.Migrate())
	require.NoError(t, s.Repo.UpsertAgentMemory(ctx, &models.AgentMemory{AgentProfileID: "chief", Layer: coordinatorMemoryLayer, Key: "k", Content: "Kept", Metadata: "{}"}))
	rows, err := s.Repo.ListAgentMemory(ctx, "chief")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "Kept", rows[0].Content)
}

func TestFreshInstallCreatesNoRetiredAssistantTables(t *testing.T) {
	_, db, _ := newRuntime(t)
	var tables int
	require.NoError(t, db.Get(&tables, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='orchestration_assistant_bindings'`))
	require.Zero(t, tables)
	var columns int
	require.NoError(t, db.Get(&columns, `SELECT count(*) FROM pragma_table_info('orchestration_memory') WHERE name IN ('owner_user_id','scope','confirmed','forgotten_at')`))
	require.Zero(t, columns)
}
