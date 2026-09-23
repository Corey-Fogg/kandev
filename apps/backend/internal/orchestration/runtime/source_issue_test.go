package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const tasksPath = "/api/v1/orchestration/runtime/tasks"

func TestCreateTaskWithSourceRecordsIssueAndReusesExistingTask(t *testing.T) {
	s, db, conversation := newRuntime(t)
	manager := &fakeTaskManager{}
	s.Manager = manager
	router, token, run := workspaceControlCaller(t, s, conversation)

	response := runtimeRequest(t, router, "POST", tasksPath, token, run,
		map[string]any{"title": "Fix login", "source": map[string]any{"tracker": "Jira", "key": "abc-12", "url": "https://example.atlassian.net/browse/ABC-12"}})
	require.Equal(t, 201, response.Code, response.Body.String())
	require.Equal(t, &models.SourceIssue{Tracker: models.TrackerJira, Key: "ABC-12", URL: "https://example.atlassian.net/browse/ABC-12"}, manager.lastSpec.Source)

	_, err := db.Exec(`INSERT INTO tasks(id,workspace_id,title,metadata,created_at,updated_at) VALUES('watched','ws','[ABC-12] Fix login','{"jira_issue_key":"ABC-12"}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	response = runtimeRequest(t, router, "POST", tasksPath, token, run,
		map[string]any{"title": "Fix login again", "source": map[string]any{"tracker": "jira", "key": "ABC-12"}})
	require.Equal(t, 200, response.Code, response.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, map[string]any{"id": "watched", duplicateKey: true, archivedKey: false}, body)
	require.EqualValues(t, 1, manager.creates.Load(), "a watch-created task for the issue is reused")

	for _, invalid := range []map[string]any{
		{"tracker": "github", "key": "ABC-12"},
		{"tracker": "linear", "key": "not a key"},
		{"tracker": "linear", "key": "ENG-3", "url": "http://insecure.example/ENG-3"},
	} {
		response = runtimeRequest(t, router, "POST", tasksPath, token, run, map[string]any{"title": "Invalid", "source": invalid})
		require.Equal(t, 422, response.Code, invalid)
	}
	response = runtimeRequest(t, router, "POST", tasksPath, token, run,
		map[string]any{"title": "Conflict", "external_id": "other", "source": map[string]any{"tracker": "linear", "key": "ENG-3"}})
	require.Equal(t, 422, response.Code)
	require.EqualValues(t, 1, manager.creates.Load())
}

func TestCreateTaskReportsDuplicateFromManager(t *testing.T) {
	s, _, conversation := newRuntime(t)
	s.Manager = &fakeTaskManager{failure: &models.DuplicateTaskError{TaskID: "existing", Archived: true}}
	router, token, run := workspaceControlCaller(t, s, conversation)
	response := runtimeRequest(t, router, "POST", tasksPath, token, run, map[string]any{"title": "Retry", "external_id": "ref-1"})
	require.Equal(t, 200, response.Code)
	require.JSONEq(t, `{"id":"existing","duplicate":true,"archived":true}`, response.Body.String())
}

func TestTaskForSourceIssuePrefersUnarchivedTask(t *testing.T) {
	s, db, _ := newRuntime(t)
	ctx := context.Background()
	_, err := db.Exec(`INSERT INTO tasks(id,workspace_id,title,metadata,external_id,archived_at,created_at,updated_at) VALUES
		('old','ws','Old','{}','linear:ENG-3',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
		('foreign','other','Foreign','{"linear_issue_identifier":"ENG-4"}',NULL,NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	id, archived, err := s.Repo.TaskForSourceIssue(ctx, "ws", models.MetaLinearIssueIdentifier, "ENG-3", "linear:ENG-3")
	require.NoError(t, err)
	require.Equal(t, "old", id)
	require.True(t, archived)
	_, err = db.Exec(`INSERT INTO tasks(id,workspace_id,title,metadata,created_at,updated_at) VALUES('live','ws','Live','{"linear_issue_identifier":"ENG-3"}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	id, archived, err = s.Repo.TaskForSourceIssue(ctx, "ws", models.MetaLinearIssueIdentifier, "ENG-3", "linear:ENG-3")
	require.NoError(t, err)
	require.Equal(t, "live", id)
	require.False(t, archived)
	id, _, err = s.Repo.TaskForSourceIssue(ctx, "ws", models.MetaLinearIssueIdentifier, "ENG-4", "linear:ENG-4")
	require.NoError(t, err)
	require.Empty(t, id, "another workspace's task is never matched")
	_, _, err = s.Repo.TaskForSourceIssue(ctx, "ws", "title') OR 1=1 --", "x", "x")
	require.Error(t, err)
}

type fakeSourceIssues struct {
	calls  []models.SourceIssueUpdate
	result models.SourceIssueResult
	err    error
}

func (f *fakeSourceIssues) UpdateSourceIssue(_ context.Context, _, _ string, update models.SourceIssueUpdate) (models.SourceIssueResult, error) {
	f.calls = append(f.calls, update)
	return f.result, f.err
}

func TestUpdateSourceIssueValidatesAndScopesToWorkspace(t *testing.T) {
	s, _, conversation := newRuntime(t)
	writer := &fakeSourceIssues{result: models.SourceIssueResult{Commented: true}}
	s.SourceIssues = writer
	tasks := s.Tasks.(*testTasks)
	tasks.tasks["delivery"] = &taskmodels.Task{ID: "delivery", WorkspaceID: "ws"}
	tasks.tasks["foreign"] = &taskmodels.Task{ID: "foreign", WorkspaceID: "other"}
	router, token, run := workspaceControlCaller(t, s, conversation)
	path := func(id string) string { return tasksPath + "/" + id + "/source-issue" }

	require.Equal(t, 404, runtimeRequest(t, router, "POST", path("foreign"), token, run, map[string]any{"comment": "Done"}).Code)
	require.Equal(t, 422, runtimeRequest(t, router, "POST", path("delivery"), token, run, map[string]any{}).Code)
	require.Equal(t, 422, runtimeRequest(t, router, "POST", path("delivery"), token, run, map[string]any{"state": "closed"}).Code)
	long := make([]byte, models.SourceCommentMaxBytes+1)
	for i := range long {
		long[i] = 'a'
	}
	require.Equal(t, 422, runtimeRequest(t, router, "POST", path("delivery"), token, run, map[string]any{"comment": string(long)}).Code)
	require.Empty(t, writer.calls)

	response := runtimeRequest(t, router, "POST", path("delivery"), token, run, map[string]any{"comment": "  Opened a pull request.  ", "state": "review"})
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Equal(t, []models.SourceIssueUpdate{{Comment: "Opened a pull request.", State: "review"}}, writer.calls)

	writer.err = &models.SourceStateError{State: "review", Available: []string{"Start (to In Progress)"}}
	response = runtimeRequest(t, router, "POST", path("delivery"), token, run, map[string]any{"state": "review"})
	require.Equal(t, 422, response.Code)
	require.Contains(t, response.Body.String(), "Start (to In Progress)")
	writer.err = models.ErrNoSourceIssue
	require.Equal(t, 422, runtimeRequest(t, router, "POST", path("delivery"), token, run, map[string]any{"state": "done"}).Code)
}
