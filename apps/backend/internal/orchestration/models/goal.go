package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Task metadata keys owned by orchestration and criterion statuses.
const (
	MetaTaskGoal  = "orchestration_goal"
	MetaTaskStall = "orchestration_stall"

	CriterionUnverified = "unverified"
	CriterionMet        = "met"
	CriterionUnmet      = "unmet"

	MaxAcceptanceCriteria     = 10
	MaxCriterionTextRunes     = 300
	MaxCriterionEvidenceRunes = 1000
)

// AcceptanceCriterion is one checkable outcome of a delegated task.
type AcceptanceCriterion struct {
	ID            string     `json:"id"`
	Text          string     `json:"text"`
	Status        string     `json:"status"`
	Evidence      string     `json:"evidence,omitempty"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
	VerifiedRunID string     `json:"verified_run_id,omitempty"`
}

// TaskGoal is a delegated task's acceptance criteria and their status.
type TaskGoal struct {
	Criteria  []AcceptanceCriterion `json:"criteria"`
	UpdatedAt time.Time             `json:"updated_at"`
}

// CriterionVerification records the coordinator's check of one criterion.
type CriterionVerification struct {
	ID       string `json:"id"`
	Met      *bool  `json:"met"`
	Evidence string `json:"evidence"`
}

// CriteriaProgress counts met criteria.
type CriteriaProgress struct {
	Met   int `json:"met"`
	Total int `json:"total"`
}

// TaskStall is the last stall episode of a delegated task.
type TaskStall struct {
	Outcome    string    `json:"outcome"`
	StalledFor string    `json:"stalled_for,omitempty"`
	SessionID  string    `json:"session_id,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
}

// ErrCriteriaUnmet means a task cannot be completed while a criterion is not met.
var ErrCriteriaUnmet = errors.New("acceptance_criteria_unmet")

// NewTaskGoal builds an unverified goal from criterion texts. It returns nil
// for an empty list.
func NewTaskGoal(texts []string, now time.Time) (*TaskGoal, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if len(texts) > MaxAcceptanceCriteria {
		return nil, fmt.Errorf("acceptance_criteria allows at most %d items", MaxAcceptanceCriteria)
	}
	goal := &TaskGoal{Criteria: make([]AcceptanceCriterion, 0, len(texts)), UpdatedAt: now.UTC()}
	for i, raw := range texts {
		text := strings.TrimSpace(raw)
		if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxCriterionTextRunes {
			return nil, fmt.Errorf("acceptance criterion %d must be 1 to %d characters", i+1, MaxCriterionTextRunes)
		}
		goal.Criteria = append(goal.Criteria, AcceptanceCriterion{ID: fmt.Sprintf("c%d", i+1), Text: text, Status: CriterionUnverified})
	}
	return goal, nil
}

// TaskGoalFromMetadata reads a task's goal. It returns nil when the task has
// none or the stored value cannot be read.
func TaskGoalFromMetadata(metadata map[string]any) *TaskGoal {
	raw, ok := metadata[MetaTaskGoal]
	if !ok || raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var goal TaskGoal
	if json.Unmarshal(data, &goal) != nil || len(goal.Criteria) == 0 {
		return nil
	}
	for i := range goal.Criteria {
		switch goal.Criteria[i].Status {
		case CriterionMet, CriterionUnmet:
		default:
			goal.Criteria[i].Status = CriterionUnverified
		}
	}
	return &goal
}

// TaskStallFromMetadata reads a task's last stall episode, or nil.
func TaskStallFromMetadata(metadata map[string]any) *TaskStall {
	raw, ok := metadata[MetaTaskStall]
	if !ok || raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var stall TaskStall
	if json.Unmarshal(data, &stall) != nil || stall.Outcome == "" {
		return nil
	}
	return &stall
}

// Progress counts the goal's met criteria.
func (g *TaskGoal) Progress() CriteriaProgress {
	progress := CriteriaProgress{Total: len(g.Criteria)}
	for _, criterion := range g.Criteria {
		if criterion.Status == CriterionMet {
			progress.Met++
		}
	}
	return progress
}

// Unmet lists the criteria not yet recorded as met.
func (g *TaskGoal) Unmet() []AcceptanceCriterion {
	var unmet []AcceptanceCriterion
	for _, criterion := range g.Criteria {
		if criterion.Status != CriterionMet {
			unmet = append(unmet, criterion)
		}
	}
	return unmet
}

// Verify records the result of checking some criteria. Every item must name
// a known criterion once, say whether it is met and carry evidence; on any
// error nothing changes.
func (g *TaskGoal) Verify(items []CriterionVerification, runID string, now time.Time) error {
	if len(items) == 0 {
		return errors.New("criteria must list at least one checked criterion")
	}
	index := make(map[string]int, len(g.Criteria))
	for i, criterion := range g.Criteria {
		index[criterion.ID] = i
	}
	seen := map[string]bool{}
	for _, item := range items {
		if _, ok := index[item.ID]; !ok || seen[item.ID] {
			return fmt.Errorf("unknown or repeated criterion id %q", item.ID)
		}
		seen[item.ID] = true
		if item.Met == nil {
			return fmt.Errorf("criterion %s requires met", item.ID)
		}
		evidence := strings.TrimSpace(item.Evidence)
		if evidence == "" || !utf8.ValidString(evidence) || utf8.RuneCountInString(evidence) > MaxCriterionEvidenceRunes {
			return fmt.Errorf("criterion %s requires evidence of 1 to %d characters", item.ID, MaxCriterionEvidenceRunes)
		}
	}
	at := now.UTC()
	for _, item := range items {
		criterion := &g.Criteria[index[item.ID]]
		criterion.Status = CriterionUnmet
		if *item.Met {
			criterion.Status = CriterionMet
		}
		criterion.Evidence = strings.TrimSpace(item.Evidence)
		criterion.VerifiedAt = &at
		criterion.VerifiedRunID = runID
	}
	g.UpdatedAt = at
	return nil
}
