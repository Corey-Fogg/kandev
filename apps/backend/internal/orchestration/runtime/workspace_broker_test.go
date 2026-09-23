package runtime

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestCoordinatorCannotTargetLinkedWorkspace(t *testing.T) {
	s, db, conversation := newRuntime(t)
	s.Manager = &workspaceGrantManager{assistantTaskManager: &assistantTaskManager{}}
	_, err := db.Exec(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('linked','Example linked workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	b := bindTestAssistant(t, s, db, "owner", "chief", conversation)
	router, token, run := workspaceControlCaller(t, s, conversation)
	path := "/api/v1/orchestration/runtime/workspace?workspace_id=linked&workspace_grant_revision=1"
	require.Equal(t, 403, runtimeRequest(t, router, "GET", path, token, run, nil).Code)
	receiver, err := s.workspaceGrantReceiver(context.Background(), b)
	require.NoError(t, err)
	g := &models.WorkspaceGrant{WorkspaceID: "linked", BindingVersion: b.Version, ReceiverProfileID: receiver.ProfileID, ReceiverProfileRevision: receiver.ProfileRevision, AuthorityRevision: receiver.AuthorityRevision, Scope: models.WorkspaceGrantScope{Operations: []string{"observe"}, ContextExports: []string{"directory"}}}
	require.NoError(t, s.Repo.SaveWorkspaceGrant(context.Background(), b, g, 0))
	response := runtimeRequest(t, router, "GET", path, token, run, nil)
	require.Equal(t, 403, response.Code, "a stored grant never widens a coordinator credential")
	require.NotContains(t, response.Body.String(), "Example linked workspace")
	require.Equal(t, 404, runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/workspace-links", token, run, nil).Code)
}
