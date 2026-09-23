package backendapp

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/config"
	officestore "github.com/kandev/kandev/internal/office/repository/sqlite"
	orchstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

func coordinatorConversationFixture(t *testing.T) (*taskCreatorAdapter, *taskservice.Service, *orchstore.Repository, string) {
	t.Helper()
	a, svc := newOfficeTaskAdapterHarness(t)
	db := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	repo := orchstore.New(db, db)
	require.NoError(t, repo.Migrate())
	ctx := context.Background()
	profiles, _, err := settingsstore.Provide(db, db, nil)
	require.NoError(t, err)
	require.NoError(t, profiles.CreateAgent(ctx, &settings.Agent{ID: "provider", Name: "provider"}))
	persona := &settings.AgentProfile{ID: "fixture-chief", AgentID: "provider", Name: "Fixture chief", Role: settings.AgentRoleAssistant, WorkspaceID: "ws-1"}
	require.NoError(t, profiles.CreateAgentProfile(ctx, persona))
	require.NoError(t, repo.RegisterOrchestrator(ctx, persona.ID, persona.WorkspaceID, "chief-of-staff"))
	conversation, err := repo.EnsureAgentConversation(ctx, persona)
	require.NoError(t, err)
	return a, svc, repo, conversation.TaskID
}

func TestUnregisteredCoordinatorKeepsOfficeBehaviour(t *testing.T) {
	a, svc, repo, _ := coordinatorConversationFixture(t)
	ctx := context.Background()
	db := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	office, err := officestore.NewWithDB(db, db, nil)
	require.NoError(t, err)
	params := routeParams{features: config.FeaturesConfig{Office: true}, officeRepo: office, orchestrationRepo: repo, taskSvc: svc}
	allowed, err := orchestrationBrowserRouteAllowed(ctx, params, "/api/v1/office/agents/fixture-chief")
	require.NoError(t, err)
	require.False(t, allowed, "Office never owns a registered coordinator")
	require.NoError(t, repo.UnregisterOrchestrator(ctx, "fixture-chief"))
	allowed, err = orchestrationBrowserRouteAllowed(ctx, params, "/api/v1/office/agents/fixture-chief")
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = orchestrationRunGuard(params.features, office)(ctx, "fixture-chief")
	require.NoError(t, err)
	require.True(t, allowed)
}

func TestCoordinatorListingExcludesConversation(t *testing.T) {
	_, svc, _, taskID := coordinatorConversationFixture(t)
	for _, query := range []string{"", "Conversation"} {
		rows, _, err := svc.ListKanbanTasksByWorkspace(context.Background(), "ws-1", models.KanbanTaskQuery{Query: query, Page: 1, PageSize: 100})
		require.NoError(t, err)
		for _, row := range rows {
			require.NotEqual(t, taskID, row.ID)
		}
	}
}
