package runtime

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
	"testing"
)

type testAssistantInputs struct {
	input   models.AttentionInput
	calls   int
	failure error
	actor   models.InputResponse
}

func (f *testAssistantInputs) ReadInput(context.Context, *models.AssistantBinding, models.Attention) (*models.AttentionInput, error) {
	value := f.input
	return &value, nil
}
func (f *testAssistantInputs) ResolveInput(_ context.Context, _ *models.AssistantBinding, _ models.Attention, response models.InputResponse) (any, error) {
	f.calls++
	f.actor = response
	if f.failure != nil {
		return nil, f.failure
	}
	f.input.State = "resolved"
	return map[string]string{"status": "resolved"}, nil
}
func assistantInputFixture(t *testing.T) (*Service, *models.AssistantBinding, models.Attention, *testAssistantInputs) {
	t.Helper()
	s, _, b, _ := assistantAttentionFixture(t)
	ctx := context.Background()
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	rows, err := s.Repo.AttentionPage(ctx, b.ID, "", 100)
	require.NoError(t, err)
	var row models.Attention
	for _, r := range rows {
		if r.Kind == "question" {
			row = r
		}
	}
	f := &testAssistantInputs{input: models.AttentionInput{SourceID: row.SourceID, Kind: "question", State: "pending", SourceRevision: row.SourceRevision, TaskID: row.TaskID, SessionID: row.SessionID, PendingID: row.SourceID,
		Questions: []models.InputQuestion{{ID: "color", Prompt: "Choose a sample color", Options: []models.InputOption{{ID: "blue", Label: "Blue"}, {ID: "green", Label: "Green"}}}}}}
	s.Inputs = f
	return s, b, row, f
}
func inputBody(row models.Attention, operation string) map[string]any {
	return map[string]any{"operation_id": operation, "expected_intent_revision": 0, "expected_binding_version": 1, "expected_revision": row.Revision, "source_revision": row.SourceRevision, "session_id": row.SessionID, "answers": []map[string]any{{"question_id": "color", "selected_options": []string{"blue"}}}}
}
func TestAssistantInputHumanNativeParity(t *testing.T) {
	s, _, row, f := assistantInputFixture(t)
	path := "/api/v1/orchestration/assistant/attention/" + row.ID + "/resolve"
	result := runtimeRequest(t, assistantRouter(s), "POST", path, "", "", inputBody(row, "answer"))
	require.Equal(t, 200, result.Code, result.Body.String())
	require.Equal(t, 1, f.calls)
	require.Equal(t, "user", f.actor.ActorType)
	require.Equal(t, "owner", f.actor.ActorID)
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "POST", path, "", "", inputBody(row, "foreign")).Code)
}
func TestAssistantResolutionIdempotencyAndExpiry(t *testing.T) {
	s, _, row, f := assistantInputFixture(t)
	router := assistantRouter(s)
	path := "/api/v1/orchestration/assistant/attention/" + row.ID + "/resolve"
	body := inputBody(row, "answer")
	first := runtimeRequest(t, router, "POST", path, "", "", body)
	require.Equal(t, 200, first.Code, first.Body.String())
	replay := runtimeRequest(t, router, "POST", path, "", "", body)
	require.Equal(t, 200, replay.Code, replay.Body.String())
	require.JSONEq(t, first.Body.String(), replay.Body.String())
	require.Equal(t, 1, f.calls)
	body["answers"] = []map[string]any{{"question_id": "color", "custom_text": "Different"}}
	require.Equal(t, 409, runtimeRequest(t, router, "POST", path, "", "", body).Code)
	body["operation_id"] = "expired"
	f.input.State = "expired"
	require.Equal(t, 409, runtimeRequest(t, router, "POST", path, "", "", body).Code)
	require.Equal(t, 1, f.calls)
}
func TestAssistantResolutionUnknownDoesNotRepeat(t *testing.T) {
	s, _, row, f := assistantInputFixture(t)
	f.failure = context.DeadlineExceeded
	router := assistantRouter(s)
	path := "/api/v1/orchestration/assistant/attention/" + row.ID + "/resolve"
	body := inputBody(row, "unknown")
	require.Equal(t, 503, runtimeRequest(t, router, "POST", path, "", "", body).Code)
	require.Equal(t, 409, runtimeRequest(t, router, "POST", path, "", "", body).Code)
	require.Equal(t, 1, f.calls)
}
func TestAssistantInputRejectsStaleAndWrongSession(t *testing.T) {
	s, _, row, f := assistantInputFixture(t)
	path := "/api/v1/orchestration/assistant/attention/" + row.ID + "/resolve"
	for key, value := range map[string]any{"expected_binding_version": 2, "expected_revision": 2, "session_id": "foreign", "source_revision": "old"} {
		body := inputBody(row, fmt.Sprint(key))
		body[key] = value
		require.Equal(t, 409, runtimeRequest(t, assistantRouter(s), "POST", path, "", "", body).Code)
	}
	require.Zero(t, f.calls)
}
