package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// promptMemoryBudgetBytes bounds the memory injected into one coordinator prompt.
const promptMemoryBudgetBytes = 12 * 1024

type promptMemorySelection struct {
	Memory        []models.ContextMemory `json:"memory"`
	OmittedMemory int                    `json:"omitted_memory"`
}

func (s *Service) promptMemory(ctx context.Context, a *models.AgentInstance, taskID string, rows []*models.AgentMemory) (*promptMemorySelection, error) {
	b, err := s.Repo.AssistantForConversation(ctx, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		b = &models.AssistantBinding{WorkspaceID: a.WorkspaceID, OrchestratorID: a.ID}
	} else if err != nil {
		return nil, err
	}
	p := &promptMemorySelection{Memory: []models.ContextMemory{}}
	return p, fillPromptMemory(p, b, rows)
}

func fillPromptMemory(p *promptMemorySelection, b *models.AssistantBinding, rows []*models.AgentMemory) error {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Confirmed != rows[j].Confirmed {
			return rows[i].Confirmed
		}
		if rows[i].Priority != rows[j].Priority {
			return rows[i].Priority > rows[j].Priority
		}
		if !rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
			return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
		}
		return rows[i].ID < rows[j].ID
	})
	for _, m := range rows {
		if !m.MatchesContext(b, models.ContextScope{}) || (m.ExpiresAt != nil && !m.ExpiresAt.After(time.Now())) {
			continue
		}
		content, truncated := memoryExcerpt(redaction.NewRedactor().String(m.Content), 1024)
		if m.Confirmed && m.Priority == 100 && truncated {
			return fmt.Errorf("required memory %s exceeds the excerpt budget", m.ID)
		}
		p.Memory = append(p.Memory, models.ContextMemory{ID: m.ID, Revision: m.Revision, Scope: m.Scope, ScopeID: m.ScopeID, SourceCommentID: m.SourceCommentID, Confirmed: m.Confirmed, Content: content, Truncated: truncated})
		if !promptMemoryFits(p) {
			p.Memory = p.Memory[:len(p.Memory)-1]
			if m.Confirmed && m.Priority == 100 {
				return fmt.Errorf("required memory cannot fit in the context budget")
			}
			p.OmittedMemory++
		}
	}
	return nil
}

func promptMemoryFits(p *promptMemorySelection) bool {
	raw, err := json.Marshal(p)
	return err == nil && len(raw) <= promptMemoryBudgetBytes-128 // reserve omission-counter growth
}

func memoryExcerpt(text string, max int) (string, bool) {
	if len(text) <= max {
		return text, false
	}
	for max > 0 && !utf8.RuneStart(text[max]) {
		max--
	}
	return text[:max], true
}
