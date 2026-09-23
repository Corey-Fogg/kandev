package runtime

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorCannotTargetForeignWorkspace(t *testing.T) {
	s, db, conversation := newRuntime(t)
	s.Manager = &fakeTaskManager{}
	_, err := db.Exec(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('linked','Example linked workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	router, token, run := workspaceControlCaller(t, s, conversation)
	response := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/workspace?workspace_id=linked", token, run, nil)
	require.Equal(t, 403, response.Code, "a coordinator credential never reaches another workspace")
	require.NotContains(t, response.Body.String(), "Example linked workspace")
	require.Equal(t, 404, runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/workspace-links", token, run, nil).Code)
}
