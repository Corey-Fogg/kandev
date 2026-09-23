package orchestration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
)

const orchestratorsPath = "/api/v1/orchestration/workspaces/ws/orchestrators"

type describedOrchestrator struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	DisplayName        string `json:"display_name"`
	RoleName           string `json:"role_name"`
	AskBeforeCreate    bool   `json:"ask_before_create"`
	AutoCommentSource  bool   `json:"auto_comment_source"`
	AutoMoveSourceDone bool   `json:"auto_move_source_done"`
}

func decodeOrchestrator(t *testing.T, body []byte) describedOrchestrator {
	t.Helper()
	var row describedOrchestrator
	require.NoError(t, json.Unmarshal(body, &row), string(body))
	return row
}

func createOrchestrator(t *testing.T, r *gin.Engine, body map[string]any) describedOrchestrator {
	t.Helper()
	cfg := map[string]any{"role_id": "chief-of-staff", "profile_id": "personal"}
	for key, value := range body {
		cfg[key] = value
	}
	w := request(t, r, http.MethodPost, orchestratorsPath, cfg)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	return decodeOrchestrator(t, w.Body.Bytes())
}

func TestCreateReturnsDefaultsAndAllowsSeveralOrchestrators(t *testing.T) {
	r, repo, db := configuredHandler(t, nil)
	first := createOrchestrator(t, r, map[string]any{"display_name": "  Jeb  ", "ask_before_create": true})
	require.Equal(t, describedOrchestrator{ID: first.ID, Name: "Jeb", DisplayName: "Jeb", RoleName: "Chief of staff", AskBeforeCreate: true, AutoCommentSource: true}, first)

	second := createOrchestrator(t, r, map[string]any{"display_name": "Val", "profile_id": "work"})
	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, "Val", second.Name)
	require.False(t, second.AskBeforeCreate, "each orchestrator keeps its own settings")
	ids, err := repo.ListOrchestratorIDs(context.Background(), "ws")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{first.ID, second.ID}, ids)

	// A legacy assistant can be imported next to the workspace's orchestrators.
	require.NoError(t, repo.UnregisterOrchestrator(context.Background(), first.ID))
	imported := request(t, r, http.MethodPost, "/api/v1/orchestration/workspaces/ws/import/"+first.ID, map[string]any{"role_id": "chief-of-staff", "profile_id": "personal"})
	require.Equal(t, http.StatusOK, imported.Code, imported.Body.String())
	var assistants int
	require.NoError(t, db.Get(&assistants, `SELECT COUNT(*) FROM workspace_orchestrators WHERE workspace_id='ws'`))
	require.Equal(t, 2, assistants)
}

func TestPatchRenamesWhileWorking(t *testing.T) {
	r, _, db := configuredHandler(t, nil)
	row := createOrchestrator(t, r, nil)
	path := orchestratorsPath + "/" + row.ID
	_, err := db.Exec(`INSERT INTO tasks VALUES('conversation','Conversation with Chief of staff');
		INSERT INTO orchestration_conversations(id,workspace_id,agent_profile_id,platform,task_id) VALUES('c','ws',?,'web','conversation');
		UPDATE agent_profiles SET status='working' WHERE id=?`, row.ID, row.ID)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, request(t, r, http.MethodPut, path, map[string]any{"role_id": "chief-of-staff", "profile_id": "personal"}).Code,
		"a full update still waits for the current turn")

	w := request(t, r, http.MethodPatch, path, map[string]any{"display_name": "Jeb"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	patched := decodeOrchestrator(t, w.Body.Bytes())
	require.Equal(t, "Jeb", patched.Name)
	require.Equal(t, "Jeb", patched.DisplayName)
	require.Equal(t, "Chief of staff", patched.RoleName)
	var name, title string
	require.NoError(t, db.Get(&name, `SELECT name FROM agent_profiles WHERE id=?`, row.ID))
	require.NoError(t, db.Get(&title, `SELECT title FROM tasks WHERE id='conversation'`))
	require.Equal(t, "Jeb", name)
	require.Equal(t, "Conversation with Jeb", title)
	listed := request(t, r, http.MethodGet, orchestratorsPath, nil)
	require.Contains(t, listed.Body.String(), `"name":"Jeb"`)

	w = request(t, r, http.MethodPatch, path, map[string]any{"auto_move_source_done": true})
	require.Equal(t, http.StatusOK, w.Code)
	patched = decodeOrchestrator(t, w.Body.Bytes())
	require.True(t, patched.AutoMoveSourceDone)
	require.True(t, patched.AutoCommentSource, "settings the patch leaves out are unchanged")
	require.Equal(t, "Jeb", patched.Name)

	for _, invalid := range []map[string]any{{}, {"display_name": strings.Repeat("J", 61)}, {"display_name": "Je\u0007b"}} {
		require.Equal(t, http.StatusBadRequest, request(t, r, http.MethodPatch, path, invalid).Code, invalid)
	}
	w = request(t, r, http.MethodPatch, path, map[string]any{"display_name": ""})
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "Chief of staff", decodeOrchestrator(t, w.Body.Bytes()).Name, "an empty name reverts to the role name")
	require.Equal(t, http.StatusNotFound, request(t, r, http.MethodPatch, "/api/v1/orchestration/workspaces/other/orchestrators/"+row.ID, map[string]any{"display_name": "X"}).Code)
}

func TestUpdateKeepsAnInstanceNameAcrossRoleEdits(t *testing.T) {
	r, _, _ := configuredHandler(t, nil)
	row := createOrchestrator(t, r, map[string]any{"display_name": "Jeb"})
	role := request(t, r, http.MethodPut, "/api/v1/orchestration/roles/chief-of-staff", map[string]any{"name": "Chief of Staff", "instructions": "Coordinate"})
	require.Equal(t, http.StatusOK, role.Code, role.Body.String())
	w := request(t, r, http.MethodPut, orchestratorsPath+"/"+row.ID, map[string]any{"role_id": "chief-of-staff", "profile_id": "work"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	updated := decodeOrchestrator(t, w.Body.Bytes())
	require.Equal(t, "Jeb", updated.Name)
	require.Equal(t, "Chief of Staff", updated.RoleName)
}

// fakeRuntime records the human route calls.
type fakeRuntime struct {
	decisions []models.ProposalDecision
	decideErr error
	proposal  *models.TaskProposal
	days      []int
}

func (f *fakeRuntime) CoordinatorMetrics(_ context.Context, _, _ string, days int) (models.CoordinatorMetrics, error) {
	f.days = append(f.days, days)
	return models.CoordinatorMetrics{Days: days, Delegated: 3}, nil
}
func (f *fakeRuntime) ListProposals(context.Context, string, string, string, int) ([]models.TaskProposal, error) {
	return []models.TaskProposal{*f.proposal}, nil
}
func (f *fakeRuntime) GetProposal(_ context.Context, _, _, id string) (*models.TaskProposal, error) {
	if id != f.proposal.ID {
		return nil, models.ErrProposalNotFound
	}
	return f.proposal, nil
}
func (f *fakeRuntime) DecideProposal(_ context.Context, d models.ProposalDecision) (*models.TaskProposal, string, bool, error) {
	f.decisions = append(f.decisions, d)
	return f.proposal, "task", false, f.decideErr
}

func TestHumanRuntimeRoutes(t *testing.T) {
	runtime := &fakeRuntime{proposal: &models.TaskProposal{ID: "p1", Status: models.ProposalPending, Spec: models.ProposalSpec{Title: "Fix login"}}}
	r, _, _ := configuredHandler(t, func(h *Handler) { h.Runtime = runtime })
	row := createOrchestrator(t, r, nil)
	base := orchestratorsPath + "/" + row.ID
	w := request(t, r, http.MethodGet, base+"/metrics?days=30", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"days":30`)
	require.Equal(t, http.StatusUnprocessableEntity, request(t, r, http.MethodGet, base+"/metrics?days=14", nil).Code)
	require.Equal(t, http.StatusNotFound, request(t, r, http.MethodGet, "/api/v1/orchestration/workspaces/other/orchestrators/"+row.ID+"/metrics", nil).Code)
	require.Equal(t, []int{30}, runtime.days)

	w = request(t, r, http.MethodGet, base+"/proposals?status=pending&limit=5", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"proposals":[{"id":"p1"`)
	require.Equal(t, http.StatusUnprocessableEntity, request(t, r, http.MethodGet, base+"/proposals?limit=51", nil).Code)
	require.Equal(t, http.StatusNotFound, request(t, r, http.MethodGet, base+"/proposals/missing", nil).Code)
	require.Equal(t, http.StatusOK, request(t, r, http.MethodGet, base+"/proposals/p1", nil).Code)

	title := "Fix the login form"
	w = request(t, r, http.MethodPost, base+"/proposals/p1/approve", map[string]any{"edits": map[string]any{"title": title}, "workspace_id": "other"})
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `"task"`, mustField(t, w.Body.Bytes(), "task_id"))
	require.Equal(t, "ws", runtime.decisions[0].WorkspaceID, "scope comes from the route")
	require.Equal(t, row.ID, runtime.decisions[0].OrchestratorID)
	require.Equal(t, title, *runtime.decisions[0].Edits.Title)
	require.Equal(t, "user-1", runtime.decisions[0].UserID)

	runtime.decideErr = models.ErrProposalDecided
	w = request(t, r, http.MethodPost, base+"/proposals/p1/dismiss", map[string]any{"reason": "Not now"})
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), `"error":"proposal_already_decided","proposal":{"id":"p1"`)
	require.Equal(t, "Not now", runtime.decisions[1].Reason)
	runtime.decideErr = &models.ProposalInputError{Err: context.DeadlineExceeded}
	require.Equal(t, http.StatusUnprocessableEntity, request(t, r, http.MethodPost, base+"/proposals/p1/approve", nil).Code)
}

func mustField(t *testing.T, body []byte, key string) string {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &fields))
	return string(fields[key])
}

func TestHumanRuntimeRoutesRequireAUserAndManageAccess(t *testing.T) {
	r, _, _ := configuredHandler(t, nil)
	row := createOrchestrator(t, r, nil)
	require.Equal(t, http.StatusServiceUnavailable, request(t, r, http.MethodGet, orchestratorsPath+"/"+row.ID+"/metrics", nil).Code, "no runtime, no metrics")

	agent := gin.New()
	RegisterRoutes(agent.Group("/api/v1/orchestration", func(c *gin.Context) { c.Set("agent_caller", true) }), &Handler{Runtime: &fakeRuntime{}})
	require.Equal(t, http.StatusForbidden, request(t, agent, http.MethodGet, orchestratorsPath+"/chief/metrics", nil).Code)

	reader := gin.New()
	noManage := func(context.Context, string) error { return errForbiddenForTest }
	RegisterRoutes(reader.Group("/api/v1/orchestration"), &Handler{Runtime: &fakeRuntime{}, Authorize: func(context.Context, string) error { return nil }, AuthorizeManage: noManage})
	for _, route := range []struct{ method, path string }{
		{http.MethodPatch, orchestratorsPath + "/chief"},
		{http.MethodPost, orchestratorsPath + "/chief/proposals/p1/approve"},
		{http.MethodPost, orchestratorsPath + "/chief/proposals/p1/dismiss"},
	} {
		require.Equal(t, http.StatusForbidden, request(t, reader, route.method, route.path, map[string]any{}).Code, route.path)
	}
}

// signedIn marks every request as made by one signed-in user.
func signedIn(c *gin.Context) { c.Set(authn.GinContextKey, authn.Identity{UserID: "user-1"}) }
