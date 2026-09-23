package jira

import (
	"context"
	"encoding/json"
	"errors"
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
	_, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, InstanceType: InstanceTypeCloud, Secret: "t",
	})
	require.NoError(t, err)
	waitForAuthProbe(t, f)
	mock.SetProjectStatuses("PROJ", []JiraStatus{
		{ID: "1", Name: "To Do", StatusCategory: "new"},
		{ID: "2", Name: "In Progress", StatusCategory: "indeterminate"},
		{ID: "3", Name: "In Review", StatusCategory: "indeterminate"},
		{ID: "4", Name: "Done", StatusCategory: "done"},
	})
	mock.AddTicket(&JiraTicket{Key: "PROJ-1", ProjectKey: "PROJ", StatusName: "To Do", StatusCategory: "new"})
	mock.AddTransitions("PROJ-1", []JiraTransition{
		{ID: "11", Name: "Start", ToStatusID: "2", ToStatusName: "In Progress"},
		{ID: "21", Name: "Submit", ToStatusID: "3", ToStatusName: "In Review"},
		{ID: "31", Name: "Finish", ToStatusID: "4", ToStatusName: "Done"},
	})
	return f.svc, mock
}

func TestAddCommentForWorkspaceUsesTheWorkspaceClient(t *testing.T) {
	svc, mock := newWritebackFixture(t)
	require.NoError(t, svc.AddCommentForWorkspace(context.Background(), "default", "PROJ-1", "Opened a pull request."))
	require.Equal(t, []string{"Opened a pull request."}, mock.Comments("PROJ-1"))
}

func TestTransitionToCategoryPicksTheHintedTransition(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, category, prefer string
		strict                 bool
		want                   string
	}{
		{"review", "indeterminate", "review", true, "PROJ-1:21"},
		{"started", "indeterminate", "progress", false, "PROJ-1:11"},
		{"done", "done", "", false, "PROJ-1:31"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, mock := newWritebackFixture(t)
			match, err := svc.TransitionToCategoryForWorkspace(ctx, "default", "PROJ-1", tc.category, tc.prefer, tc.strict)
			require.NoError(t, err)
			require.NotNil(t, match.Transition)
			calls := mock.TransitionCalls()
			require.Len(t, calls, 1)
			require.Equal(t, tc.want, calls[0].TicketKey+":"+calls[0].TransitionID)
		})
	}
}

func TestTransitionToCategoryReportsCurrentAndMissingTransitions(t *testing.T) {
	ctx := context.Background()
	svc, mock := newWritebackFixture(t)
	mock.AddTicket(&JiraTicket{Key: "PROJ-1", ProjectKey: "PROJ", StatusName: "Done", StatusCategory: "done"})
	match, err := svc.TransitionToCategoryForWorkspace(ctx, "default", "PROJ-1", "done", "", false)
	require.NoError(t, err)
	require.True(t, match.Current)

	mock.AddTicket(&JiraTicket{Key: "PROJ-2", ProjectKey: "PROJ", StatusName: "To Do", StatusCategory: "new"})
	mock.AddTransitions("PROJ-2", []JiraTransition{{ID: "11", Name: "Start", ToStatusID: "2", ToStatusName: "In Progress"}})
	match, err = svc.TransitionToCategoryForWorkspace(ctx, "default", "PROJ-2", "indeterminate", "review", true)
	require.NoError(t, err)
	require.Nil(t, match.Transition, "a strict hint never falls back to another status")
	require.Equal(t, []string{"Start (to In Progress)"}, match.Available)
	require.Empty(t, mock.TransitionCalls())
}

func TestCloudClientAddCommentBodies(t *testing.T) {
	for _, tc := range []struct {
		instance, path string
		body           any
	}{
		{InstanceTypeCloud, "/rest/api/3/issue/PROJ-1/comment", map[string]any{"type": "doc", "version": float64(1), "content": []any{
			map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "Line one"}}},
			map[string]any{"type": "paragraph"},
			map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "Line two"}}},
		}}},
		{InstanceTypeServer, "/rest/api/2/issue/PROJ-1/comment", "Line one\n\nLine two"},
	} {
		t.Run(tc.instance, func(t *testing.T) {
			var got map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, tc.path, r.URL.Path)
				raw, _ := io.ReadAll(r.Body)
				require.NoError(t, json.Unmarshal(raw, &got))
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":"1"}`))
			}))
			defer server.Close()
			client := NewCloudClient(&JiraConfig{SiteURL: server.URL, AuthMethod: AuthMethodPAT, InstanceType: tc.instance}, "token")
			require.NoError(t, client.AddComment(context.Background(), "PROJ-1", "Line one\n\nLine two"))
			require.Equal(t, tc.body, got["body"])
		})
	}
}

func TestAddCommentForWorkspaceRejectsClientsWithoutComments(t *testing.T) {
	f := newSvcFixture(t)
	_, err := f.svc.SetConfig(context.Background(), &SetConfigRequest{
		SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, InstanceType: InstanceTypeCloud, Secret: "t",
	})
	require.NoError(t, err)
	waitForAuthProbe(t, f)
	err = f.svc.AddCommentForWorkspace(context.Background(), "default", "PROJ-1", "text")
	require.True(t, errors.Is(err, ErrCommentUnsupported), err)
}
