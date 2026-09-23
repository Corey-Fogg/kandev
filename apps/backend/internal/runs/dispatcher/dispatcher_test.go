package dispatcher

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/runs/models"
)

type queueFake struct {
	rows     []*models.Run
	failed   []string
	released []string
}

func (q *queueFake) ClaimNextEligibleRun(context.Context) (*models.Run, error) {
	if len(q.rows) == 0 {
		return nil, sql.ErrNoRows
	}
	r := q.rows[0]
	q.rows = q.rows[1:]
	return r, nil
}
func (q *queueFake) FinishRun(_ context.Context, id, status string, _ *string) (*models.Run, error) {
	q.failed = append(q.failed, id+":"+status)
	return nil, nil
}
func (q *queueFake) UpdateRunOutputSummary(context.Context, string, string, string) error { return nil }
func (q *queueFake) ReleaseClaim(_ context.Context, id string) error {
	q.released = append(q.released, id)
	return nil
}
func TestDispatcherHasOneOwnerPerRun(t *testing.T) {
	q := &queueFake{rows: []*models.Run{{ID: "conversation"}, {ID: "task"}, {ID: "unsupported"}}}
	var calls []string
	d := Dispatcher{Queue: q, Handlers: []Handler{
		func(_ context.Context, r *models.Run) (bool, error) {
			if r.ID == "conversation" {
				calls = append(calls, r.ID)
				return true, nil
			}
			return false, nil
		},
		func(_ context.Context, r *models.Run) (bool, error) {
			calls = append(calls, r.ID)
			if r.ID == "unsupported" {
				return true, fmt.Errorf("disabled")
			}
			return true, nil
		},
	}}
	d.Tick(context.Background())
	require.Equal(t, []string{"conversation", "task", "unsupported"}, calls)
	require.Equal(t, []string{"unsupported:failed"}, q.failed)
}

func TestDispatcherReleasesARunWhoseOwnerIsUnknown(t *testing.T) {
	q := &queueFake{rows: []*models.Run{{ID: "undecided"}, {ID: "next"}}}
	var calls []string
	d := Dispatcher{Queue: q, Handlers: []Handler{
		func(_ context.Context, r *models.Run) (bool, error) {
			if r.ID == "undecided" {
				return false, fmt.Errorf("lookup: %w", ErrOwnerUnknown)
			}
			return false, nil
		},
		func(_ context.Context, r *models.Run) (bool, error) {
			calls = append(calls, r.ID)
			return true, nil
		},
	}}
	d.Tick(context.Background())
	require.Empty(t, calls, "an undecided run is neither failed nor handed to the next runtime")
	require.Empty(t, q.failed)
	require.Equal(t, []string{"undecided"}, q.released)
	require.Len(t, q.rows, 1, "the tick stops instead of reclaiming the released run")
}
