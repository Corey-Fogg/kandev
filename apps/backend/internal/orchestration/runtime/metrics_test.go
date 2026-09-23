package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func seedDelegatedOutcome(t *testing.T, db *sqlx.DB, id, chief, state string, created, updated time.Time) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO tasks(id,workspace_id,title,state,metadata,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		id, "ws", id, state, `{"orchestration_chief_id":"`+chief+`"}`, created, updated)
	require.NoError(t, err)
}

func seedUsage(t *testing.T, db *sqlx.DB, id, task, source string, subcents int64, at time.Time) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO task_usage_events(usage_event_id,task_id,cost_subcents,cost_source,contract_version,occurred_at,created_at) VALUES(?,?,?,?,1,?,?)`,
		id, task, subcents, source, at, at)
	require.NoError(t, err)
}

func TestCoordinatorMetricsSummariseDelegatedOutcomes(t *testing.T) {
	s, db, conversation := newRuntime(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	day := func(n int) time.Time { return now.Add(-time.Duration(n) * 24 * time.Hour) }
	seedDelegatedOutcome(t, db, "merged", "chief", "COMPLETED", day(3), day(1))
	seedDelegatedOutcome(t, db, "completed", "chief", "COMPLETED", day(2), day(2).Add(4*time.Hour))
	seedDelegatedOutcome(t, db, "failed", "chief", "FAILED", day(1), day(1))
	seedDelegatedOutcome(t, db, "open", "chief", "IN_PROGRESS", day(1), day(1))
	seedDelegatedOutcome(t, db, "old", "chief", "COMPLETED", day(20), day(19))
	seedDelegatedOutcome(t, db, "other", "someone-else", "COMPLETED", day(1), day(1))

	result, err := s.coordinatorMetrics(ctx, "ws", "chief", conversation, 7, now)
	require.NoError(t, err)
	require.Equal(t, 4, result.Delegated)
	require.Equal(t, 2, result.Completed)
	require.Equal(t, 1, result.Failed)
	require.InDelta(t, 0.67, *result.SuccessRate, 0.001)
	require.Nil(t, result.MergedPullRequests, "merges are unknown without the pull request store")
	require.Equal(t, 2, result.CycleTimeSamples)

	_, err = db.Exec(`CREATE TABLE github_task_prs (task_id TEXT, merged_at DATETIME)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO github_task_prs(task_id,merged_at) VALUES('merged',?),('merged',?),('other',?)`, day(2), day(1), day(1))
	require.NoError(t, err)
	seedUsage(t, db, "u1", "merged", "provider_reported", 25000, day(2))
	seedUsage(t, db, "u2", conversation, "provider_reported", 5000, day(1))
	seedUsage(t, db, "u3", "failed", "unpriced", 0, day(1))
	seedUsage(t, db, "u4", "other", "provider_reported", 90000, day(1))
	seedUsage(t, db, "u5", "merged", "provider_reported", 70000, day(9))

	result, err = s.coordinatorMetrics(ctx, "ws", "chief", conversation, 7, now)
	require.NoError(t, err)
	require.Equal(t, 1, *result.MergedPullRequests)
	require.Equal(t, 4.0, *result.CycleTimeMedianHours)
	require.Equal(t, 24.0, *result.CycleTimeP90Hours, "the first merge ends the cycle")
	require.Equal(t, 3.0, result.CostUSD, "the coordinator's own conversation counts; other coordinators and older usage do not")
	require.Equal(t, 3.0, *result.CostPerMergedPRUSD)
	require.Equal(t, 1, result.UnpricedEvents)

	result, err = s.coordinatorMetrics(ctx, "ws", "chief", conversation, 30, now)
	require.NoError(t, err)
	require.Equal(t, 5, result.Delegated)
	require.Equal(t, 10.0, result.CostUSD)
}

func TestMetricsRouteAcceptsSevenOrThirtyDays(t *testing.T) {
	s, _, conversation := newRuntime(t)
	router, token, run := workspaceControlCaller(t, s, conversation)
	response := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/metrics?days=30", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.EqualValues(t, 30, body["days"])
	require.EqualValues(t, 0, body["delegated"])
	require.Nil(t, body["success_rate"])
	require.Equal(t, 422, runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/metrics?days=90", token, run, nil).Code)
}

func TestPercentileUsesNearestRank(t *testing.T) {
	require.Nil(t, percentile(nil, 0.5))
	values := []float64{10, 1, 4, 2, 8, 6, 3, 9, 5, 7}
	require.Equal(t, 5.0, *percentile(values, 0.5))
	require.Equal(t, 9.0, *percentile(values, 0.9))
}
