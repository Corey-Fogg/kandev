package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestration/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakeTaskMetadata stores orchestration metadata on the in-memory tasks.
type fakeTaskMetadata struct {
	tasks  *testTasks
	writes int
}

func (f *fakeTaskMetadata) SetTaskMetadata(_ context.Context, taskID, key string, value any) (bool, error) {
	task := f.tasks.tasks[taskID]
	if task == nil || task.ArchivedAt != nil {
		return false, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return false, err
	}
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	task.Metadata[key] = decoded
	f.writes++
	return true, nil
}

func withTaskMetadata(s *Service) *fakeTaskMetadata {
	metadata := &fakeTaskMetadata{tasks: s.Tasks.(*testTasks)}
	s.TaskMetadata = metadata
	return metadata
}

func manageTaskPath(id string) string { return tasksPath + "/" + id + "/manage" }

func TestCreateTaskValidatesAndStoresAcceptanceCriteria(t *testing.T) {
	s, _, conversation := newRuntime(t)
	manager := &fakeTaskManager{}
	s.Manager = manager
	router, token, run := workspaceControlCaller(t, s, conversation)
	eleven := make([]string, 11)
	for i := range eleven {
		eleven[i] = "Criterion"
	}
	for _, invalid := range [][]string{eleven, {""}, {"ok", "  "}, {strings.Repeat("é", 301)}} {
		response := runtimeRequest(t, router, "POST", tasksPath, token, run, map[string]any{"title": "Task", "acceptance_criteria": invalid})
		require.Equal(t, 422, response.Code, response.Body.String())
	}
	require.Zero(t, manager.creates.Load())
	response := runtimeRequest(t, router, "POST", tasksPath, token, run, map[string]any{"title": "Task", "acceptance_criteria": []string{" Tests pass ", "Docs updated"}})
	require.Equal(t, 201, response.Code, response.Body.String())
	goal := manager.lastSpec.Goal
	require.NotNil(t, goal)
	require.Equal(t, []models.AcceptanceCriterion{{ID: "c1", Text: "Tests pass", Status: "unverified"}, {ID: "c2", Text: "Docs updated", Status: "unverified"}}, goal.Criteria)
	response = runtimeRequest(t, router, "POST", tasksPath, token, run, map[string]any{"title": "No criteria"})
	require.Equal(t, 201, response.Code)
	require.Nil(t, manager.lastSpec.Goal)
}

func TestCriteriaActionsAreScopedAndGateCompletion(t *testing.T) {
	s, _, conversation := newRuntime(t)
	s.Manager = &fakeTaskManager{}
	metadata := withTaskMetadata(s)
	var statusCalls []string
	s.UpdateStatus = func(_ context.Context, _, id, status string) error {
		statusCalls = append(statusCalls, id+":"+status)
		return nil
	}
	task := delegate(s, "delegated", v1.TaskStateReview)
	other := delegate(s, "foreign-chief", v1.TaskStateReview)
	other.Metadata["orchestration_chief_id"] = "someone-else"
	router, token, run := workspaceControlCaller(t, s, conversation)
	manage := func(id string, body map[string]any) (int, string) {
		response := runtimeRequest(t, router, "POST", manageTaskPath(id), token, run, body)
		return response.Code, response.Body.String()
	}
	code, _ := manage("delegated", map[string]any{"action": "verify_criteria", "criteria": []any{}})
	require.Equal(t, 422, code, "a task without criteria has nothing to verify")
	code, body := manage("delegated", map[string]any{"action": "set_criteria", "acceptance_criteria": []string{"Tests pass", "Docs updated"}})
	require.Equal(t, 200, code, body)
	code, _ = manage("foreign-chief", map[string]any{"action": "set_criteria", "acceptance_criteria": []string{"x"}})
	require.Equal(t, 403, code)
	code, _ = manage("foreign-chief", map[string]any{"action": "verify_criteria", "criteria": []map[string]any{{"id": "c1", "met": true, "evidence": "x"}}})
	require.Equal(t, 403, code)

	writes := metadata.writes
	for _, invalid := range [][]map[string]any{
		{{"id": "c9", "met": true, "evidence": "Checked"}},
		{{"id": "c1", "met": true, "evidence": ""}},
		{{"id": "c1", "evidence": "No verdict"}},
		{{"id": "c1", "met": true, "evidence": "Fine"}, {"id": "c3", "met": true, "evidence": "Unknown"}},
	} {
		code, _ = manage("delegated", map[string]any{"action": "verify_criteria", "criteria": invalid})
		require.Equal(t, 422, code, invalid)
	}
	require.Equal(t, writes, metadata.writes, "a rejected verification changes nothing")
	require.Equal(t, models.CriteriaProgress{Met: 0, Total: 2}, models.TaskGoalFromMetadata(task.Metadata).Progress())

	done := func() (int, string) {
		response := runtimeRequest(t, router, "POST", tasksPath+"/delegated/status", token, run, map[string]any{"status": "done"})
		return response.Code, response.Body.String()
	}
	code, body = done()
	require.Equal(t, 409, code)
	require.Contains(t, body, `"error":"acceptance_criteria_unmet"`)
	require.Contains(t, body, `"id":"c1"`)
	require.Empty(t, statusCalls)

	code, body = manage("delegated", map[string]any{"action": "verify_criteria", "criteria": []map[string]any{
		{"id": "c1", "met": true, "evidence": "go test ./... passed"}, {"id": "c2", "met": false, "evidence": "README unchanged"}}})
	require.Equal(t, 200, code, body)
	require.Contains(t, body, `"met":1`)
	code, body = done()
	require.Equal(t, 409, code)
	require.Contains(t, body, `"id":"c2"`)
	require.NotContains(t, body, `"id":"c1"`)

	code, _ = manage("delegated", map[string]any{"action": "verify_criteria", "criteria": []map[string]any{{"id": "c2", "met": true, "evidence": "README section added"}}})
	require.Equal(t, 200, code)
	goal := models.TaskGoalFromMetadata(task.Metadata)
	require.Equal(t, run, goal.Criteria[1].VerifiedRunID)
	require.NotNil(t, goal.Criteria[1].VerifiedAt)
	code, _ = done()
	require.Equal(t, 200, code)
	require.Equal(t, []string{"delegated:done"}, statusCalls)

	code, _ = manage("delegated", map[string]any{"action": "set_criteria", "acceptance_criteria": []string{"Tests pass", "Changelog entry"}})
	require.Equal(t, 200, code)
	require.Equal(t, models.CriteriaProgress{Met: 0, Total: 2}, models.TaskGoalFromMetadata(task.Metadata).Progress(), "set_criteria resets every criterion")
	code, body = manage("delegated", map[string]any{"action": "set_criteria", "acceptance_criteria": []string{}})
	require.Equal(t, 409, code, "clearing unmet criteria would lift the completion gate")
	require.Contains(t, body, `"error":"acceptance_criteria_unmet"`)
	require.NotNil(t, models.TaskGoalFromMetadata(task.Metadata))
	code, _ = manage("delegated", map[string]any{"action": "verify_criteria", "criteria": []map[string]any{
		{"id": "c1", "met": true, "evidence": "go test ./... passed"}, {"id": "c2", "met": true, "evidence": "CHANGELOG entry added"}}})
	require.Equal(t, 200, code)
	code, _ = manage("delegated", map[string]any{"action": "set_criteria", "acceptance_criteria": []string{}})
	require.Equal(t, 200, code)
	require.Nil(t, models.TaskGoalFromMetadata(task.Metadata), "an empty list clears criteria that are all met")
}

// lockedTaskMetadata is a fakeTaskMetadata safe for concurrent writers.
type lockedTaskMetadata struct {
	mu sync.Mutex
	*fakeTaskMetadata
}

func (f *lockedTaskMetadata) SetTaskMetadata(ctx context.Context, taskID, key string, value any) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fakeTaskMetadata.SetTaskMetadata(ctx, taskID, key, value)
}

func TestParallelVerificationsKeepEveryVerdict(t *testing.T) {
	s, _, conversation := newRuntime(t)
	s.Manager = &fakeTaskManager{}
	s.TaskMetadata = &lockedTaskMetadata{fakeTaskMetadata: &fakeTaskMetadata{tasks: s.Tasks.(*testTasks)}}
	task := delegate(s, "delegated", v1.TaskStateReview)
	goal, err := models.NewTaskGoal([]string{"One", "Two", "Three", "Four"}, s.now())
	require.NoError(t, err)
	task.Metadata[models.MetaTaskGoal] = goal
	router, token, run := workspaceControlCaller(t, s, conversation)
	var wg sync.WaitGroup
	codes := make([]int, 4)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := map[string]any{"action": "verify_criteria", "criteria": []map[string]any{{"id": fmt.Sprintf("c%d", i+1), "met": true, "evidence": "Checked"}}}
			codes[i] = runtimeRequest(t, router, "POST", manageTaskPath("delegated"), token, run, body).Code
		}(i)
	}
	wg.Wait()
	require.Equal(t, []int{200, 200, 200, 200}, codes)
	require.Equal(t, models.CriteriaProgress{Met: 4, Total: 4}, models.TaskGoalFromMetadata(task.Metadata).Progress(), "no verdict is lost")
}

func TestVerifyEvidenceIsRedacted(t *testing.T) {
	s, _, conversation := newRuntime(t)
	withTaskMetadata(s)
	task := delegate(s, "delegated", v1.TaskStateReview)
	router, token, run := workspaceControlCaller(t, s, conversation)
	require.Equal(t, 200, runtimeRequest(t, router, "POST", manageTaskPath("delegated"), token, run, map[string]any{"action": "set_criteria", "acceptance_criteria": []string{"Deployed"}}).Code)
	secret := "ghp_" + strings.Repeat("a", 36)
	require.Equal(t, 200, runtimeRequest(t, router, "POST", manageTaskPath("delegated"), token, run, map[string]any{"action": "verify_criteria",
		"criteria": []map[string]any{{"id": "c1", "met": true, "evidence": "Used token " + secret}}}).Code)
	require.NotContains(t, models.TaskGoalFromMetadata(task.Metadata).Criteria[0].Evidence, secret)
}

func TestTaskUpdatesCarryAcceptanceCriteria(t *testing.T) {
	s, db, _ := newRuntime(t)
	task := delegate(s, "delegated", v1.TaskStateReview)
	goal, err := models.NewTaskGoal([]string{"Tests pass", "Docs updated"}, s.now())
	require.NoError(t, err)
	met := true
	require.NoError(t, goal.Verify([]models.CriterionVerification{{ID: "c1", Met: &met, Evidence: "CI green"}}, "run", s.now()))
	task.Metadata[models.MetaTaskGoal] = goal
	require.NoError(t, s.taskCallback(context.Background(), "delegated"))
	ids := queuedCallbacks(t, db)
	require.Len(t, ids, 1)
	updates := queuedUpdates(t, s, ids[0])
	require.Equal(t, []criterionDigest{{ID: "c1", Text: "Tests pass", Status: "met"}, {ID: "c2", Text: "Docs updated", Status: "unverified"}}, updates[0].Criteria)

	var text strings.Builder
	writeTaskUpdates(&text, updates)
	require.Contains(t, text.String(), "  Acceptance criteria (1/2 met): c1 met; c2 unverified: Docs updated\n")
	require.Contains(t, text.String(), "Verify each criterion with manage_task verify_criteria before reporting this task done.")

	require.NoError(t, goal.Verify([]models.CriterionVerification{{ID: "c2", Met: &met, Evidence: "Docs merged"}}, "run", s.now()))
	task.Metadata[models.MetaTaskGoal] = goal
	require.NoError(t, s.taskCallback(context.Background(), "delegated"))
	require.Len(t, queuedCallbacks(t, db), 1, "criteria are not part of the digest identity")

	text.Reset()
	writeTaskUpdates(&text, []taskUpdate{{TaskID: "t", Title: "T", State: "IN_PROGRESS", Criteria: []criterionDigest{{ID: "c1", Text: "x", Status: "unverified"}}}})
	require.NotContains(t, text.String(), "before reporting this task done", "the reminder is only for review and completion")
}

func TestTaskDetailsSummaryCriteriaAreReadTolerantly(t *testing.T) {
	require.Nil(t, models.TaskGoalFromMetadata(map[string]any{models.MetaTaskGoal: "not a goal"}))
	require.Nil(t, models.TaskGoalFromMetadata(map[string]any{models.MetaTaskGoal: nil}))
	goal := models.TaskGoalFromMetadata(map[string]any{models.MetaTaskGoal: map[string]any{"criteria": []any{map[string]any{"id": "c1", "text": "x", "status": "bogus"}}}})
	require.Equal(t, models.CriterionUnverified, goal.Criteria[0].Status)
}
