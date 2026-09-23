package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

type brokerDelayedTransport struct{ calls int }

func (r *brokerDelayedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r.calls++
	timer := time.NewTimer(31 * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"accepted"}`)), Request: request}, nil
	case <-request.Context().Done():
		return nil, request.Context().Err()
	}
}

func TestOrchestratorBrokerWaitsForColdSessionResumeWithoutRetry(t *testing.T) {
	for _, action := range []string{"start", "message"} {
		t.Run(action, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				transport := &brokerDelayedTransport{}
				client := &kandevClient{apiURL: "http://example.invalid", http: &http.Client{Timeout: 30 * time.Second, Transport: transport}}
				result, err := callOrchestratorBroker(client, models.WorkspaceBrokerTool{Name: "manage_task", Method: http.MethodPost, Path: "/runtime/tasks/:id/manage"}, map[string]any{"id": "sample", "request": map[string]any{"action": action}})
				require.NoError(t, err)
				require.False(t, result.IsError, "%+v", result.Content)
				require.Equal(t, 1, transport.calls)
				require.Equal(t, 30*time.Second, client.http.Timeout, "shared read client must retain its deadline")
			})
		})
	}
}

func TestOrchestratorBrokerPreservesReadDeadlineAndDoesNotRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		transport := &brokerDelayedTransport{}
		client := &kandevClient{apiURL: "http://example.invalid", http: &http.Client{Timeout: 30 * time.Second, Transport: transport}}
		result, err := callOrchestratorBroker(client, models.WorkspaceBrokerTool{Name: "task_details", Method: http.MethodGet, Path: "/runtime/tasks/:id/details"}, map[string]any{"id": "sample"})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Equal(t, 1, transport.calls)
	})
}
