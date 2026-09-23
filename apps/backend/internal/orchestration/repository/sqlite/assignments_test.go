package sqlite

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/testutil"
)

const assignmentFixture = `CREATE TABLE workspaces(id TEXT PRIMARY KEY);
CREATE TABLE tasks(id TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '');
CREATE TABLE task_comments(id TEXT PRIMARY KEY, task_id TEXT, author_type TEXT, author_id TEXT, body TEXT, source TEXT, created_at TIMESTAMP);
CREATE TABLE agent_profiles(id TEXT PRIMARY KEY, workspace_id TEXT, name TEXT, role TEXT, deleted_at TEXT)`

// assignmentRepo opens an orchestration store over the minimal core tables it
// references, on SQLite or, with dsn, on an isolated Postgres schema.
func assignmentRepo(t *testing.T, dsn string) (*Repository, *sqlx.DB) {
	t.Helper()
	var database *sqlx.DB
	if dsn != "" {
		database = testutil.OpenIsolatedPostgres(t, dsn)
	} else {
		var err error
		database, err = sqlx.Open("sqlite3", ":memory:")
		require.NoError(t, err)
		database.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = database.Close() })
		_, err = database.Exec("PRAGMA foreign_keys=ON")
		require.NoError(t, err)
	}
	for _, statement := range splitStatements(assignmentFixture) {
		_, err := database.Exec(statement)
		require.NoError(t, err)
	}
	for _, statement := range []string{
		"INSERT INTO workspaces VALUES('ws'),('other')",
		"INSERT INTO agent_profiles VALUES('chief','ws','Chief of staff','assistant',NULL),('second','ws','Second','assistant',NULL),('elsewhere','other','Elsewhere','assistant',NULL)",
	} {
		_, err := database.Exec(statement)
		require.NoError(t, err)
	}
	repo := New(database, database)
	require.NoError(t, repo.Migrate())
	return repo, database
}

func splitStatements(script string) []string {
	var out []string
	start := 0
	for i := range script {
		if script[i] == ';' {
			out = append(out, script[start:i])
			start = i + 1
		}
	}
	return append(out, script[start:])
}

func forEachDialect(t *testing.T, run func(t *testing.T, dsn string)) {
	t.Run("sqlite", func(t *testing.T) { run(t, "") })
	if dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN"); dsn != "" {
		t.Run("postgres", func(t *testing.T) { run(t, dsn) })
	}
}

func TestAssignmentMigrationsReplayWithDefaults(t *testing.T) {
	forEachDialect(t, func(t *testing.T, dsn string) {
		repo, database := assignmentRepo(t, dsn)
		ctx := context.Background()
		require.NoError(t, repo.RegisterOrchestrator(ctx, "chief", "ws", "chief-of-staff"))
		require.NoError(t, repo.Migrate(), "migrations replay on the same database")
		a, err := repo.OrchestratorAssignment(ctx, "chief")
		require.NoError(t, err)
		require.Equal(t, &models.OrchestratorAssignment{AgentID: "chief", WorkspaceID: "ws", RoleID: "chief-of-staff",
			OrchestratorSettings: models.DefaultOrchestratorSettings()}, a)
		for _, table := range []string{"orchestration_task_proposals", "orchestration_source_writebacks"} {
			var count int
			require.NoError(t, database.Get(&count, "SELECT COUNT(*) FROM "+table), table)
		}
		missing, err := repo.OrchestratorAssignment(ctx, "second")
		require.NoError(t, err)
		require.Nil(t, missing)
	})
}

func TestOneOrchestratorPerWorkspace(t *testing.T) {
	forEachDialect(t, func(t *testing.T, dsn string) {
		repo, _ := assignmentRepo(t, dsn)
		ctx := context.Background()
		require.NoError(t, repo.RegisterOrchestrator(ctx, "chief", "ws", "chief-of-staff"))
		require.NoError(t, repo.RegisterOrchestrator(ctx, "chief", "ws", "chief-of-staff"), "re-registering the same orchestrator is allowed")
		require.ErrorIs(t, repo.RegisterOrchestrator(ctx, "second", "ws", "chief-of-staff"), models.ErrOrchestratorExists)
		require.NoError(t, repo.RegisterOrchestrator(ctx, "elsewhere", "other", "chief-of-staff"))
		require.Error(t, repo.RegisterOrchestrator(ctx, "elsewhere", "ws", "chief-of-staff"))
	})
}

func TestSingleOrchestratorIndexWaitsForLegacyDuplicates(t *testing.T) {
	repo, database := assignmentRepo(t, "")
	ctx := context.Background()
	indexExists := func() bool {
		var count int
		require.NoError(t, database.Get(&count, `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='ux_workspace_orchestrators_one_per_workspace'`))
		return count == 1
	}
	require.True(t, indexExists())
	_, err := database.Exec(`DROP INDEX ux_workspace_orchestrators_one_per_workspace`)
	require.NoError(t, err)
	// An earlier build allowed several orchestrators in one workspace.
	_, err = database.Exec(`INSERT INTO workspace_orchestrators(agent_id,workspace_id,role_id) VALUES('chief','ws','chief-of-staff'),('second','ws','chief-of-staff')`)
	require.NoError(t, err)
	require.NoError(t, repo.Migrate())
	require.False(t, indexExists(), "the index is skipped while a workspace has two orchestrators")
	require.NoError(t, repo.UpdateOrchestratorRole(ctx, "second", "chief-of-staff"), "legacy duplicates stay editable")
	require.NoError(t, repo.UnregisterOrchestrator(ctx, "second"))
	require.NoError(t, repo.Migrate())
	require.True(t, indexExists(), "a replay creates the index once the duplicate is gone")
}

func TestDisplayNameRenamesProfileAndConversation(t *testing.T) {
	repo, database := assignmentRepo(t, "")
	ctx := context.Background()
	require.NoError(t, repo.RegisterOrchestrator(ctx, "chief", "ws", "chief-of-staff"))
	require.NoError(t, repo.RegisterOrchestrator(ctx, "elsewhere", "other", "chief-of-staff"))
	_, err := database.Exec(`INSERT INTO tasks(id,title) VALUES('conversation','Conversation with Chief of staff');
		INSERT INTO orchestration_conversations(id,workspace_id,agent_profile_id,platform,task_id) VALUES('c','ws','chief','web','conversation')`)
	require.NoError(t, err)

	effective, err := repo.SetOrchestratorDisplayName(ctx, "chief", "Jeb")
	require.NoError(t, err)
	require.Equal(t, "Jeb", effective)
	var name, title string
	require.NoError(t, database.Get(&name, `SELECT name FROM agent_profiles WHERE id='chief'`))
	require.NoError(t, database.Get(&title, `SELECT title FROM tasks WHERE id='conversation'`))
	require.Equal(t, "Jeb", name)
	require.Equal(t, "Conversation with Jeb", title)

	role, err := repo.GetOrchestratorRole(ctx, "chief-of-staff")
	require.NoError(t, err)
	role.Name = "Chief of Staff"
	require.NoError(t, repo.SaveOrchestratorRole(ctx, role))
	require.NoError(t, database.Get(&name, `SELECT name FROM agent_profiles WHERE id='chief'`))
	require.Equal(t, "Jeb", name, "a role rename never overwrites an instance name")
	require.NoError(t, database.Get(&name, `SELECT name FROM agent_profiles WHERE id='elsewhere'`))
	require.Equal(t, "Chief of Staff", name, "an inherited name follows the role")

	effective, err = repo.SetOrchestratorDisplayName(ctx, "chief", "")
	require.NoError(t, err)
	require.Equal(t, "Chief of Staff", effective, "an empty instance name reverts to the role name")
	require.NoError(t, database.Get(&title, `SELECT title FROM tasks WHERE id='conversation'`))
	require.Equal(t, "Conversation with Chief of Staff", title)
	conversation, err := repo.ConversationTaskID(ctx, "chief")
	require.NoError(t, err)
	require.Equal(t, "conversation", conversation)
	none, err := repo.ConversationTaskID(ctx, "elsewhere")
	require.NoError(t, err)
	require.Empty(t, none)
}

func TestSettingsRoundTrip(t *testing.T) {
	repo, _ := assignmentRepo(t, "")
	ctx := context.Background()
	require.NoError(t, repo.RegisterOrchestrator(ctx, "chief", "ws", "chief-of-staff"))
	want := models.OrchestratorSettings{AskBeforeCreate: true, AutoCommentSource: false, AutoMoveSourceDone: true}
	require.NoError(t, repo.SaveOrchestratorSettings(ctx, "chief", want))
	a, err := repo.OrchestratorAssignment(ctx, "chief")
	require.NoError(t, err)
	require.Equal(t, want, a.OrchestratorSettings)
	require.Error(t, repo.SaveOrchestratorSettings(ctx, "second", want), "an unregistered profile has no settings")
}

func TestProposalStoreIsIdempotentAndCascades(t *testing.T) {
	forEachDialect(t, func(t *testing.T, dsn string) {
		repo, database := assignmentRepo(t, dsn)
		ctx := context.Background()
		require.NoError(t, repo.RegisterOrchestrator(ctx, "chief", "ws", "chief-of-staff"))
		spec := models.ProposalSpec{Title: "Fix login", Source: &models.SourceIssue{Tracker: models.TrackerJira, Key: "ABC-1"}}
		proposal := func(id string) *models.TaskProposal {
			return &models.TaskProposal{ID: id, OrchestratorID: "chief", WorkspaceID: "ws", ConversationTaskID: "conversation", RunID: "run",
				RequestHash: spec.RequestHash(), SourceKey: "jira:ABC-1", Spec: spec}
		}
		first, created, err := repo.CreateProposal(ctx, proposal("p1"), &models.TaskComment{TaskID: "conversation", AuthorType: "agent", AuthorID: "chief", Body: "Proposed", Source: "proposal"})
		require.NoError(t, err)
		require.True(t, created)
		replay, created, err := repo.CreateProposal(ctx, proposal("p2"), &models.TaskComment{TaskID: "conversation", AuthorType: "agent", AuthorID: "chief", Body: "Proposed", Source: "proposal"})
		require.NoError(t, err)
		require.False(t, created)
		require.Equal(t, first.ID, replay.ID)
		require.Equal(t, spec, replay.Spec)
		var comments int
		require.NoError(t, database.Get(&comments, `SELECT COUNT(*) FROM task_comments WHERE id='p1' AND source='proposal'`))
		require.Equal(t, 1, comments)

		pending, err := repo.PendingProposalForSource(ctx, "chief", "jira:ABC-1")
		require.NoError(t, err)
		require.Equal(t, "p1", pending.ID)
		claimed, err := repo.ClaimProposalApproval(ctx, "p1")
		require.NoError(t, err)
		require.True(t, claimed)
		dismissed, err := repo.DismissProposal(ctx, "p1", "", "user", time.Now())
		require.NoError(t, err)
		require.False(t, dismissed, "an approval in flight cannot be dismissed")
		final := spec
		final.Title = "Fix the login form"
		done, err := repo.CompleteProposalApproval(ctx, "p1", "task", false, true, &final, "user", time.Now())
		require.NoError(t, err)
		require.True(t, done)
		stored, err := repo.GetProposal(ctx, "chief", "p1")
		require.NoError(t, err)
		require.Equal(t, models.ProposalApproved, stored.Status)
		require.Equal(t, &final, stored.FinalSpec)
		require.True(t, stored.Edited)
		require.NotNil(t, stored.DecidedAt)
		pending, err = repo.PendingProposalForSource(ctx, "chief", "jira:ABC-1")
		require.NoError(t, err)
		require.Nil(t, pending)

		require.NoError(t, repo.UnregisterOrchestrator(ctx, "chief"))
		rows, err := repo.ListProposals(ctx, "chief", "", 50)
		require.NoError(t, err)
		require.Empty(t, rows, "deleting the orchestrator deletes its proposals")
	})
}

func TestObserveTaskStateClaimsOncePerTransition(t *testing.T) {
	forEachDialect(t, func(t *testing.T, dsn string) {
		repo, database := assignmentRepo(t, dsn)
		ctx := context.Background()
		_, err := database.Exec(`INSERT INTO tasks(id) VALUES('task')`)
		require.NoError(t, err)
		observe := func(state string, reportable bool) (int64, bool) {
			t.Helper()
			episode, claimed, err := repo.ObserveTaskState(ctx, "task", state, reportable)
			require.NoError(t, err)
			return episode, claimed
		}
		episode, claimed := observe("REVIEW", true)
		require.True(t, claimed)
		require.EqualValues(t, 1, episode)
		_, claimed = observe("REVIEW", true)
		require.False(t, claimed, "a redelivered state claims nothing")
		require.NoError(t, repo.FinishSourceWriteBack(ctx, "task", 1, WriteBackPosted, true, "", ""))
		_, claimed = observe("IN_PROGRESS", false)
		require.False(t, claimed)
		episode, claimed = observe("REVIEW", true)
		require.True(t, claimed, "returning to review is a new transition")
		require.EqualValues(t, 3, episode)
		row, err := repo.GetSourceWriteBack(ctx, "task")
		require.NoError(t, err)
		require.Equal(t, WriteBackClaimed, row.Status)
		require.False(t, row.Commented, "a new episode resets the previous outcome")
	})
}
