// Package dispatcher owns the single durable run claim loop. Features provide handlers.
package dispatcher

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/runs/models"
)

// ErrOwnerUnknown tells the dispatcher a handler could not decide whether it
// owns a run, for example because a lookup failed. The claim is released for
// a later tick instead of failing the run or handing it to another runtime.
var ErrOwnerUnknown = errors.New("run owner is unknown")

type Queue interface {
	ClaimNextEligibleRun(context.Context) (*models.Run, error)
	FinishRun(context.Context, string, string, *string) (*models.Run, error)
	UpdateRunOutputSummary(context.Context, string, string, string) error
	ReleaseClaim(context.Context, string) error
}
type Handler func(context.Context, *models.Run) (bool, error)
type Dispatcher struct {
	Queue    Queue
	Handlers []Handler
	Before   func(context.Context)
	After    func(context.Context)
	OnError  func(error)
}

func (d *Dispatcher) report(err error) {
	if err != nil && d.OnError != nil {
		d.OnError(err)
	}
}
func (d *Dispatcher) Tick(ctx context.Context) {
	if d.Before != nil {
		d.Before(ctx)
	}
	for i := 0; i < 10 && ctx.Err() == nil; i++ {
		run, err := d.Queue.ClaimNextEligibleRun(ctx)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && run == nil) {
			break
		}
		if err != nil {
			d.report(err)
			break
		}
		if released := d.dispatch(ctx, run); released {
			// Claiming again this tick would pick the same run straight back up.
			break
		}
	}
	if d.After != nil {
		d.After(ctx)
	}
}

// dispatch hands a claimed run to its owner and reports whether the claim was
// released instead.
func (d *Dispatcher) dispatch(ctx context.Context, run *models.Run) bool {
	for _, handler := range d.Handlers {
		handled, err := handler(ctx, run)
		if errors.Is(err, ErrOwnerUnknown) {
			d.report(err)
			d.report(d.Queue.ReleaseClaim(ctx, run.ID))
			return true
		}
		if err != nil {
			d.fail(ctx, run, err)
			return false
		}
		if handled {
			return false
		}
	}
	d.fail(ctx, run, fmt.Errorf("no enabled runtime handles this run"))
	return false
}
func (d *Dispatcher) fail(ctx context.Context, run *models.Run, err error) {
	d.report(err)
	d.report(d.Queue.UpdateRunOutputSummary(ctx, run.ID, "", err.Error()))
	_, finishErr := d.Queue.FinishRun(ctx, run.ID, "failed", nil)
	d.report(finishErr)
}
