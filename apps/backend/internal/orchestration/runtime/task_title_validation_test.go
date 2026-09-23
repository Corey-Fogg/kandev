package runtime

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestCreateTaskShortensLongTitle(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &fakeTaskManager{}
	s.Manager = manager
	router, token, runID := workspaceControlCaller(t, s, task)
	long := strings.TrimSpace(strings.Repeat("Qualys inspector ", 5))
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, map[string]any{
		"title": long, "description": "Inspect the scanner.",
	})
	require.Equal(t, 201, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"title_truncated":true`)
	require.EqualValues(t, 1, manager.creates.Load())
	require.NoError(t, taskservice.ValidateTaskTitle(manager.lastSpec.Title))
	require.Equal(t, "Qualys inspector Qualys inspector Qualys inspector Qualys…", manager.lastSpec.Title)
	require.Equal(t, "Full title: "+long+"\n\nInspect the scanner.", manager.lastSpec.Description)
}

func TestFitTaskTitle(t *testing.T) {
	for name, tc := range map[string]struct {
		in, want  string
		truncated bool
	}{
		"fits":          {in: "  Short title ", want: "Short title"},
		"word boundary": {in: "Prioridex: add Category field with MSP-vendor-aligned picklist", want: "Prioridex: add Category field with MSP-vendor-aligned…", truncated: true},
		"no boundary":   {in: strings.Repeat("x", 70), want: strings.Repeat("x", 59) + "…", truncated: true},
		"multibyte":     {in: strings.Repeat("é", 61), want: strings.Repeat("é", 59) + "…", truncated: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, truncated := fitTaskTitle(tc.in)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.truncated, truncated)
			require.LessOrEqual(t, utf8.RuneCountInString(got), taskservice.TaskTitleMaxLength)
		})
	}
}
