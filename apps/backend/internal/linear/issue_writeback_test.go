package linear

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func newWritebackFixture(t *testing.T) (*Service, *MockClient) {
	t.Helper()
	f := newSvcFixture(t)
	mock := NewMockClient()
	f.svc.clientFn = MockClientFactory(mock)
	_, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{AuthMethod: AuthMethodAPIKey, Secret: "t"})
	require.NoError(t, err)
	waitForAuthProbe(t, f)
	mock.SetStates("ENG", []LinearWorkflowState{
		{ID: "s-review", Name: "In Review", Type: "started", Position: 2},
		{ID: "s-progress", Name: "In Progress", Type: "started", Position: 1},
		{ID: "s-done", Name: "Done", Type: "completed", Position: 3},
		{ID: "s-todo", Name: "Todo", Type: "unstarted", Position: 0},
	})
	mock.AddIssue(&LinearIssue{ID: "uuid-1", Identifier: "ENG-1", TeamKey: "ENG", StateName: "Todo", StateType: "unstarted"})
	return f.svc, mock
}

func TestAddCommentForWorkspaceAddressesTheIssueUUID(t *testing.T) {
	svc, mock := newWritebackFixture(t)
	require.NoError(t, svc.AddCommentForWorkspace(context.Background(), "default", "ENG-1", "Opened a pull request."))
	require.Equal(t, []string{"Opened a pull request."}, mock.Comments("uuid-1"))
}

func TestMoveToStateTypePicksTheHintedState(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, stateType, prefer, want string
		strict                        bool
	}{
		{"review", "started", "review", "s-review", true},
		{"started", "started", "progress", "s-progress", false},
		{"done", "completed", "", "s-done", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, mock := newWritebackFixture(t)
			match, err := svc.MoveToStateTypeForWorkspace(ctx, "default", "ENG-1", tc.stateType, tc.prefer, tc.strict)
			require.NoError(t, err)
			require.Equal(t, tc.want, match.State.ID)
			require.Equal(t, []setStateCall{{IssueID: "uuid-1", StateID: tc.want}}, mock.SetStateCalls())
		})
	}
}

func TestMoveToStateTypeReportsCurrentAndMissingStates(t *testing.T) {
	ctx := context.Background()
	svc, mock := newWritebackFixture(t)
	mock.AddIssue(&LinearIssue{ID: "uuid-2", Identifier: "ENG-2", TeamKey: "ENG", StateName: "Done", StateType: "completed"})
	match, err := svc.MoveToStateTypeForWorkspace(ctx, "default", "ENG-2", "completed", "", false)
	require.NoError(t, err)
	require.True(t, match.Current)
	mock.SetStates("ENG", []LinearWorkflowState{{ID: "s-progress", Name: "In Progress", Type: "started"}})
	match, err = svc.MoveToStateTypeForWorkspace(ctx, "default", "ENG-1", "started", "review", true)
	require.NoError(t, err)
	require.Nil(t, match.State)
	require.Equal(t, []string{"In Progress (started)"}, match.Available)
	require.Empty(t, mock.SetStateCalls())
}

func TestGraphQLClientAddComment(t *testing.T) {
	var got struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &got))
		_, _ = w.Write([]byte(`{"data":{"commentCreate":{"success":true}}}`))
	}))
	defer server.Close()
	client := NewGraphQLClient(nil, "key")
	client.endpoint = server.URL
	require.NoError(t, client.AddComment(context.Background(), "uuid-1", "Done."))
	require.Contains(t, got.Query, "commentCreate")
	require.Equal(t, map[string]any{"issueId": "uuid-1", "body": "Done."}, got.Variables)
}
