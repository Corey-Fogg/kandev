package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantMemoryOwnerConfirmationAndForget(t *testing.T) {
	s, db, task := newRuntime(t)
	bindTestAssistant(t, s, db, "owner", "chief", task)
	router := assistantRouter(s)
	path := "/api/v1/orchestration/assistant/memory/preference"
	request := map[string]any{"key": "concise", "content": "Use short updates", "scope": "workspace", "source_comment_id": "source", "expected_revision": 0, "confirmed": true}
	require.NoError(t, s.Repo.PutComment(context.Background(), &models.TaskComment{ID: "source", TaskID: task, AuthorType: "user", AuthorID: "owner", Body: "Use short updates", Source: "user"}))
	response := runtimeRequest(t, router, "PUT", path, "", "", request)
	require.Equal(t, 200, response.Code, response.Body.String())
	var row map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &row))
	require.Equal(t, "owner", row["owner_user_id"])
	require.Equal(t, true, row["confirmed"])
	require.EqualValues(t, 1, row["revision"])
	require.Equal(t, 409, runtimeRequest(t, router, "PUT", path, "", "", request).Code)
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "GET", path, "", "", nil).Code)
	require.Equal(t, 200, runtimeRequest(t, router, "DELETE", path, "", "", map[string]any{"expected_revision": 1}).Code)
	list := runtimeRequest(t, router, "GET", "/api/v1/orchestration/assistant/memory", "", "", nil)
	require.Equal(t, 200, list.Code, list.Body.String())
	require.NotContains(t, list.Body.String(), "Use short updates")
}
