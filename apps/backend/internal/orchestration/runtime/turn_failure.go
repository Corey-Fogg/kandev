package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestration/models"
	runmodels "github.com/kandev/kandev/internal/runs/models"
)

// unboundRunTimeout bounds how long a claimed coordinator turn may go without
// binding a session before it is failed and becomes retryable.
const unboundRunTimeout = 5 * time.Minute

const turnFailureSource = "runtime"

// FailUnboundRuns fails coordinator turns that were claimed but never bound a
// session, so a stalled launch cannot hold the coordinator's queue. A session
// that binds later is refused by the broker and the dispatch guard because its
// run is no longer claimed.
func (s *Service) FailUnboundRuns(ctx context.Context, now time.Time) error {
	rows, err := s.Repo.UnboundClaimedRuns(ctx, now.Add(-unboundRunTimeout))
	if err != nil {
		return err
	}
	for _, run := range rows {
		reason := fmt.Sprintf("the coordinator session did not start within %s", unboundRunTimeout)
		if err := s.Runs.RecordFailure(ctx, run.ID, reason); err != nil {
			return err
		}
		finished, err := s.Runs.FinishRun(ctx, run.ID, statusFailed, nil)
		if err != nil {
			return err
		}
		if finished == nil {
			continue
		}
		s.retiredExecutions.Delete(run.ID)
		if err := s.Repo.SetRuntimeWorking(ctx, run.AgentProfileID, false); err != nil {
			return err
		}
		if err := s.postTurnFailure(ctx, run, reason); err != nil {
			return err
		}
	}
	return nil
}

// postTurnFailure tells the conversation that a turn ended without a reply,
// once per run, so the user sees why and can retry instead of re-sending.
func (s *Service) postTurnFailure(ctx context.Context, run *runmodels.Run, reason string) error {
	var payload struct {
		TaskID string `json:"task_id"`
	}
	if json.Unmarshal([]byte(run.Payload), &payload) != nil || payload.TaskID == "" {
		return nil
	}
	owner, _, err := s.Repo.ConversationOwner(ctx, payload.TaskID)
	if err != nil || owner != run.AgentProfileID {
		return nil
	}
	reason, _ = clipRunes(reason, 300)
	body := fmt.Sprintf("This turn stopped before replying: %s. Use Retry on the message, or send it again.", reason)
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("turn-failure:"+run.ID)).String()
	return s.Repo.PutComment(ctx, &models.TaskComment{ID: id, TaskID: payload.TaskID, AuthorID: owner, AuthorType: authorTypeAgent, Body: body, Source: turnFailureSource})
}

// clipRunes cuts value to at most limit characters.
func clipRunes(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]), true
}
