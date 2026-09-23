package models

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeDisplayName(t *testing.T) {
	for raw, want := range map[string]string{"  Jeb ": "Jeb", "": "", "   ": "", strings.Repeat("é", 60): strings.Repeat("é", 60)} {
		got, err := NormalizeDisplayName(raw)
		require.NoError(t, err, raw)
		require.Equal(t, want, got)
	}
	for _, raw := range []string{strings.Repeat("x", 61), "Je\nb", "Je\x00b", string([]byte{0xff})} {
		_, err := NormalizeDisplayName(raw)
		require.ErrorIs(t, err, ErrInvalidDisplayName, raw)
	}
	require.Equal(t, "Jeb", EffectiveName("Jeb", "Chief of Staff"))
	require.Equal(t, "Chief of Staff", EffectiveName("", "Chief of Staff"))
}

func TestGoalVerificationIsAllOrNothing(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	goal, err := NewTaskGoal([]string{"One", "Two"}, now)
	require.NoError(t, err)
	yes, no := true, false
	err = goal.Verify([]CriterionVerification{{ID: "c1", Met: &yes, Evidence: "ok"}, {ID: "c2", Met: &no, Evidence: " "}}, "run", now)
	require.Error(t, err)
	require.Equal(t, CriteriaProgress{Met: 0, Total: 2}, goal.Progress(), "a rejected verification changes nothing")
	require.Error(t, goal.Verify([]CriterionVerification{{ID: "c1", Met: &yes, Evidence: "a"}, {ID: "c1", Met: &yes, Evidence: "b"}}, "run", now))
	require.Error(t, goal.Verify(nil, "run", now))
	require.NoError(t, goal.Verify([]CriterionVerification{{ID: "c1", Met: &yes, Evidence: "ok"}, {ID: "c2", Met: &no, Evidence: "missing"}}, "run", now))
	require.Equal(t, CriteriaProgress{Met: 1, Total: 2}, goal.Progress())
	require.Equal(t, []AcceptanceCriterion{{ID: "c2", Text: "Two", Status: CriterionUnmet, Evidence: "missing", VerifiedAt: &now, VerifiedRunID: "run"}}, goal.Unmet())
	empty, err := NewTaskGoal(nil, now)
	require.NoError(t, err)
	require.Nil(t, empty)
}

func TestProposalSpecApplyReportsChanges(t *testing.T) {
	spec := ProposalSpec{Title: "Fix", AcceptanceCriteria: []string{"a"}}
	same, changed := spec.Apply(&ProposalEdits{Title: ptr("Fix"), AcceptanceCriteria: &[]string{"a"}})
	require.False(t, changed)
	require.Equal(t, spec, same)
	edited, changed := spec.Apply(&ProposalEdits{Description: ptr("More"), AcceptanceCriteria: &[]string{}})
	require.True(t, changed)
	require.Equal(t, "More", edited.Description)
	require.Empty(t, edited.AcceptanceCriteria)
	require.Equal(t, []string{"a"}, spec.AcceptanceCriteria, "the original spec is untouched")
	require.NotEqual(t, spec.RequestHash(), edited.RequestHash())
	_, changed = spec.Apply(nil)
	require.False(t, changed)
}

func ptr(value string) *string { return &value }
