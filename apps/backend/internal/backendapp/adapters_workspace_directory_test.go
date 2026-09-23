package backendapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestWorkspaceCatalogIsCompactUnlessFullDetailIsRequested(t *testing.T) {
	a, _ := newWorkspaceAdminHarness(t)
	ctx := context.Background()
	wf := adminCommand(t, a, "workflow", "create", "", map[string]any{"name": "Synthetic delivery"}).(*models.Workflow)
	step := adminCommand(t, a, "step", "create", "", map[string]any{"workflow_id": wf.ID, "name": "Backlog", "is_start_step": true, "prompt": "Long step prompt"}).(*wfmodels.WorkflowStep)

	compact, err := a.WorkspaceCatalog(ctx, "ws-1", false)
	require.NoError(t, err)
	catalog := compact.(map[string]any)
	require.NotContains(t, catalog, "workflow_templates")
	require.Contains(t, catalog["workflow_steps"], catalogStep{ID: step.ID, WorkflowID: wf.ID, Name: "Backlog", Position: step.Position, IsStartStep: true, AllowManualMove: step.AllowManualMove})

	full, err := a.WorkspaceCatalog(ctx, "ws-1", true)
	require.NoError(t, err)
	require.Contains(t, full.(map[string]any), "workflow_templates")

	directory, err := a.WorkspaceDirectory(ctx, "ws-1")
	require.NoError(t, err)
	var found bool
	for _, workflow := range directory.Workflows {
		if workflow.ID == wf.ID {
			found = true
			require.Len(t, workflow.Steps, 1)
			require.Equal(t, step.ID, workflow.Steps[0].ID)
			require.True(t, workflow.Steps[0].Start)
		}
	}
	require.True(t, found, "the directory lists the delivery workflow with its steps")
}
