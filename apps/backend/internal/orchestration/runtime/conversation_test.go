package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestCoordinatorCommentReadsAreBoundedAndClipped(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	s.Tasks.(*testTasks).tasks[task] = &taskmodels.Task{ID: task, WorkspaceID: "ws"}
	long := strings.Repeat("é", runtimeCommentBodyRunes+20)
	require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{ID: "long", TaskID: task, AuthorType: authorTypeUser, Body: long}))
	for i := 0; i < runtimeCommentLimit+2; i++ {
		require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{ID: fmt.Sprintf("short-%02d", i), TaskID: task, AuthorType: authorTypeUser, Body: "Short"}))
	}
	router, token, run := workspaceControlCaller(t, s, task)
	path := "/api/v1/orchestration/tasks/" + task + "/comments"

	var page struct {
		Comments []struct {
			ID        string `json:"id"`
			Body      string `json:"body"`
			Truncated bool   `json:"truncated"`
		} `json:"comments"`
		NextCursor string `json:"next_cursor"`
	}
	response := runtimeRequest(t, router, "GET", path, token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Len(t, page.Comments, runtimeCommentLimit)
	require.NotEmpty(t, page.NextCursor)

	response = runtimeRequest(t, router, "GET", path+"?limit=50", token, run, nil)
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	var clipped bool
	for _, row := range page.Comments {
		if row.ID == "long" {
			clipped = row.Truncated && utf8.RuneCountInString(row.Body) == runtimeCommentBodyRunes
		}
	}
	require.True(t, clipped, "a long body is clipped and flagged")
	require.Equal(t, 400, runtimeRequest(t, router, "GET", path+"?limit=51", token, run, nil).Code)

	var full struct {
		Comment models.TaskComment `json:"comment"`
	}
	response = runtimeRequest(t, router, "GET", path+"?comment_id=long", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &full))
	require.Equal(t, long, full.Comment.Body)
	require.Equal(t, 404, runtimeRequest(t, router, "GET", path+"?comment_id=missing", token, run, nil).Code)
}
