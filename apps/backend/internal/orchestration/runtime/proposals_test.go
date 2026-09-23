package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	settings "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// askBeforeCreate turns proposals on for the test coordinator.
func askBeforeCreate(t *testing.T, s *Service) {
	t.Helper()
	settings := models.DefaultOrchestratorSettings()
	settings.AskBeforeCreate = true
	require.NoError(t, s.Repo.SaveOrchestratorSettings(context.Background(), "chief", settings))
}

func proposeViaBroker(t *testing.T, s *Service, conversation string, body map[string]any) (int, map[string]any) {
	t.Helper()
	router, token, run := workspaceControlCaller(t, s, conversation)
	response := runtimeRequest(t, router, "POST", tasksPath, token, run, body)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &decoded), response.Body.String())
	return response.Code, decoded
}

func queuedDecisionRuns(t *testing.T, db *sqlx.DB) []struct {
	Key     string `db:"idempotency_key"`
	Payload string `db:"payload"`
} {
	t.Helper()
	var rows []struct {
		Key     string `db:"idempotency_key"`
		Payload string `db:"payload"`
	}
	require.NoError(t, db.Select(&rows, `SELECT idempotency_key,payload FROM runs WHERE reason=? ORDER BY requested_at,id`, proposalDecisionReason))
	return rows
}

func TestCreateTaskProposesWhenAskBeforeCreateIsOn(t *testing.T) {
	s, db, conversation := newRuntime(t)
	manager := &fakeTaskManager{}
	s.Manager = manager
	askBeforeCreate(t, s)
	router, token, run := workspaceControlCaller(t, s, conversation)
	body := map[string]any{"title": "Fix login", "description": "Repair the form", "acceptance_criteria": []string{"Tests pass"}}

	response := runtimeRequest(t, router, "POST", tasksPath, token, run, body)
	require.Equal(t, 202, response.Code, response.Body.String())
	var first map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &first))
	require.Equal(t, "pending", first["status"])
	require.Equal(t, "Fix login", first[titleKey])
	require.Zero(t, manager.creates.Load(), "a proposal creates no task")

	id := first[proposalIDKey].(string)
	comment, err := s.Repo.GetCommentByID(context.Background(), conversation, id)
	require.NoError(t, err)
	require.Equal(t, "proposal", comment.Source)
	require.Equal(t, authorTypeAgent, comment.AuthorType)
	require.Contains(t, comment.Body, "**Proposed task:** Fix login")

	replay := runtimeRequest(t, router, "POST", tasksPath, token, run, body)
	require.Equal(t, 200, replay.Code, "a replay in the same run answers with the stored proposal")
	require.Contains(t, replay.Body.String(), id)
	var count int
	require.NoError(t, db.Get(&count, `SELECT COUNT(*) FROM orchestration_task_proposals`))
	require.Equal(t, 1, count)

	stored, err := s.Repo.GetProposal(context.Background(), "chief", id)
	require.NoError(t, err)
	require.Equal(t, []string{"Tests pass"}, stored.Spec.AcceptanceCriteria)
}

func TestProposalsDeduplicateBySourceIssue(t *testing.T) {
	s, db, conversation := newRuntime(t)
	manager := &fakeTaskManager{}
	s.Manager = manager
	askBeforeCreate(t, s)
	router, token, run := workspaceControlCaller(t, s, conversation)
	source := map[string]any{"tracker": "jira", "key": "ABC-12"}
	first := runtimeRequest(t, router, "POST", tasksPath, token, run, map[string]any{"title": "Fix login", "source": source})
	require.Equal(t, 202, first.Code)
	again := runtimeRequest(t, router, "POST", tasksPath, token, run, map[string]any{"title": "Fix login differently", "source": source})
	require.Equal(t, 200, again.Code)
	require.Contains(t, again.Body.String(), `"duplicate_proposal":true`)

	_, err := db.Exec(`INSERT INTO tasks(id,workspace_id,title,metadata,created_at,updated_at) VALUES('watched','ws','Watched','{"jira_issue_key":"ENG-4"}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	existing := runtimeRequest(t, router, "POST", tasksPath, token, run, map[string]any{"title": "Other", "source": map[string]any{"tracker": "jira", "key": "ENG-4"}})
	require.Equal(t, 200, existing.Code)
	require.JSONEq(t, `{"id":"watched","duplicate":true,"archived":false}`, existing.Body.String())
	var count int
	require.NoError(t, db.Get(&count, `SELECT COUNT(*) FROM orchestration_task_proposals`))
	require.Equal(t, 1, count, "an issue that already has a task is never proposed")
	require.Zero(t, manager.creates.Load())
}

func TestApprovingAProposalCreatesOneTask(t *testing.T) {
	s, db, conversation := newRuntime(t)
	manager := &fakeTaskManager{}
	s.Manager = manager
	askBeforeCreate(t, s)
	_, body := proposeViaBroker(t, s, conversation, map[string]any{"title": "Fix login", "acceptance_criteria": []string{"Tests pass", "Docs updated"}})
	id := body[proposalIDKey].(string)
	decision := models.ProposalDecision{WorkspaceID: "ws", OrchestratorID: "chief", ProposalID: id, UserID: "user", Action: models.ProposalActionApprove}

	p, taskID, duplicate, err := s.DecideProposal(context.Background(), decision)
	require.NoError(t, err)
	require.Equal(t, "created-task", taskID)
	require.False(t, duplicate)
	require.Equal(t, models.ProposalApproved, p.Status)
	require.Equal(t, "user", p.DecidedBy)
	require.EqualValues(t, 1, manager.creates.Load())
	require.Equal(t, "chief", manager.lastSpec.ChiefID)
	require.Equal(t, "orchestration-proposal:"+id, manager.lastSpec.ExternalID)
	require.Equal(t, "Fix login", manager.lastSpec.Title)
	require.Len(t, manager.lastSpec.Goal.Criteria, 2)
	require.Equal(t, "c2", manager.lastSpec.Goal.Criteria[1].ID)

	again, taskID, _, err := s.DecideProposal(context.Background(), decision)
	require.NoError(t, err)
	require.Equal(t, "created-task", taskID)
	require.Equal(t, models.ProposalApproved, again.Status)
	require.EqualValues(t, 1, manager.creates.Load(), "approving twice creates one task")

	runs := queuedDecisionRuns(t, db)
	require.Len(t, runs, 1, "the coordinator is woken once")
	require.Equal(t, "proposal-decision:"+id, runs[0].Key)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(runs[0].Payload), &payload))
	decisions := payloadProposalDecisions(payload)
	require.Equal(t, []proposalDecisionUpdate{{ProposalID: id, Outcome: "approved", Title: "Fix login", TaskID: "created-task"}}, decisions)

	_, _, _, err = s.DecideProposal(context.Background(), models.ProposalDecision{WorkspaceID: "ws", OrchestratorID: "chief", ProposalID: id, Action: models.ProposalActionDismiss})
	require.ErrorIs(t, err, models.ErrProposalDecided, "an approved proposal cannot be dismissed")
}

func TestApprovingASourcedProposalUsesTheIssueIdentity(t *testing.T) {
	s, _, conversation := newRuntime(t)
	manager := &fakeTaskManager{}
	s.Manager = manager
	askBeforeCreate(t, s)
	_, body := proposeViaBroker(t, s, conversation, map[string]any{"title": "Fix login", "source": map[string]any{"tracker": "linear", "key": "ENG-3"}})
	_, _, _, err := s.DecideProposal(context.Background(), models.ProposalDecision{WorkspaceID: "ws", OrchestratorID: "chief", ProposalID: body[proposalIDKey].(string), Action: models.ProposalActionApprove})
	require.NoError(t, err)
	require.Equal(t, "linear:ENG-3", manager.lastSpec.ExternalID)
	require.Equal(t, &models.SourceIssue{Tracker: models.TrackerLinear, Key: "ENG-3"}, manager.lastSpec.Source)
}

func TestApprovingWithEditsRecordsTheFinalSpec(t *testing.T) {
	s, db, conversation := newRuntime(t)
	manager := &fakeTaskManager{}
	s.Manager = manager
	askBeforeCreate(t, s)
	_, body := proposeViaBroker(t, s, conversation, map[string]any{"title": "Fix login", "description": "Original"})
	id := body[proposalIDKey].(string)
	title, criteria := "Fix the login form", []string{"Form submits"}
	p, _, _, err := s.DecideProposal(context.Background(), models.ProposalDecision{WorkspaceID: "ws", OrchestratorID: "chief", ProposalID: id,
		Action: models.ProposalActionApprove, Edits: &models.ProposalEdits{Title: &title, AcceptanceCriteria: &criteria}})
	require.NoError(t, err)
	require.True(t, p.Edited)
	require.NotNil(t, p.FinalSpec)
	require.Equal(t, title, p.FinalSpec.Title)
	require.Equal(t, "Original", p.FinalSpec.Description)
	require.Equal(t, "Fix login", p.Spec.Title, "the proposed spec is kept")
	require.Equal(t, title, manager.lastSpec.Title)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(queuedDecisionRuns(t, db)[0].Payload), &payload))
	require.Equal(t, "edited", payloadProposalDecisions(payload)[0].Outcome)
}

func TestFailedApprovalLeavesTheProposalPending(t *testing.T) {
	s, _, conversation := newRuntime(t)
	manager := &fakeTaskManager{failure: errors.New("select workflow_id explicitly")}
	s.Manager = manager
	askBeforeCreate(t, s)
	_, body := proposeViaBroker(t, s, conversation, map[string]any{"title": "Fix login"})
	id := body[proposalIDKey].(string)
	decision := models.ProposalDecision{WorkspaceID: "ws", OrchestratorID: "chief", ProposalID: id, Action: models.ProposalActionApprove}
	p, _, _, err := s.DecideProposal(context.Background(), decision)
	var input *models.ProposalInputError
	require.ErrorAs(t, err, &input)
	require.Equal(t, models.ProposalPending, p.Status)
	stored, err := s.Repo.GetProposal(context.Background(), "chief", id)
	require.NoError(t, err)
	require.Equal(t, models.ProposalPending, stored.Status)

	long := strings.Repeat("x", 61)
	_, _, _, err = s.DecideProposal(context.Background(), models.ProposalDecision{WorkspaceID: "ws", OrchestratorID: "chief", ProposalID: id,
		Action: models.ProposalActionApprove, Edits: &models.ProposalEdits{Title: &long}})
	require.ErrorAs(t, err, &input, "a person's title is never shortened")
	require.EqualValues(t, 1, manager.creates.Load())
}

func TestDismissIsIdempotentAndBlocksApproval(t *testing.T) {
	s, db, conversation := newRuntime(t)
	manager := &fakeTaskManager{}
	s.Manager = manager
	askBeforeCreate(t, s)
	_, body := proposeViaBroker(t, s, conversation, map[string]any{"title": "Fix login"})
	id := body[proposalIDKey].(string)
	dismiss := models.ProposalDecision{WorkspaceID: "ws", OrchestratorID: "chief", ProposalID: id, UserID: "user", Action: models.ProposalActionDismiss, Reason: "Not now"}
	p, _, _, err := s.DecideProposal(context.Background(), dismiss)
	require.NoError(t, err)
	require.Equal(t, models.ProposalDismissed, p.Status)
	require.Equal(t, "Not now", p.DismissReason)
	_, _, _, err = s.DecideProposal(context.Background(), dismiss)
	require.NoError(t, err, "dismissing twice is idempotent")
	_, _, _, err = s.DecideProposal(context.Background(), models.ProposalDecision{WorkspaceID: "ws", OrchestratorID: "chief", ProposalID: id, Action: models.ProposalActionApprove})
	require.ErrorIs(t, err, models.ErrProposalDecided)
	require.Zero(t, manager.creates.Load())
	runs := queuedDecisionRuns(t, db)
	require.Len(t, runs, 1)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(runs[0].Payload), &payload))
	require.Equal(t, []proposalDecisionUpdate{{ProposalID: id, Outcome: "dismissed", Title: "Fix login", Reason: "Not now"}}, payloadProposalDecisions(payload))

	var prompt strings.Builder
	writeProposalDecisions(&prompt, payloadProposalDecisions(payload))
	require.Contains(t, prompt.String(), `"Fix login" (proposal_id=`+id+`): dismissed. Reason: "Not now"`)
	require.Contains(t, prompt.String(), "Do not propose a dismissed task again unless the user asks.")
}

func TestDecisionOnAPausedCoordinatorIsRecordedWithoutAWake(t *testing.T) {
	s, db, conversation := newRuntime(t)
	s.Manager = &fakeTaskManager{}
	askBeforeCreate(t, s)
	_, body := proposeViaBroker(t, s, conversation, map[string]any{"title": "Fix login"})
	id := body[proposalIDKey].(string)
	_, err := db.Exec(`UPDATE runs SET status='finished'`)
	require.NoError(t, err)
	_, err = s.Personas.UpdateAgentStatus(context.Background(), "chief", settings.AgentStatusPaused, "")
	require.NoError(t, err)
	p, taskID, _, err := s.DecideProposal(context.Background(), models.ProposalDecision{WorkspaceID: "ws", OrchestratorID: "chief", ProposalID: id, Action: models.ProposalActionApprove})
	require.NoError(t, err)
	require.Equal(t, models.ProposalApproved, p.Status)
	require.Equal(t, "created-task", taskID)
	require.Empty(t, queuedDecisionRuns(t, db))
}

func TestProposalScopeRejectsOtherWorkspaces(t *testing.T) {
	s, _, conversation := newRuntime(t)
	s.Manager = &fakeTaskManager{}
	askBeforeCreate(t, s)
	_, body := proposeViaBroker(t, s, conversation, map[string]any{"title": "Fix login"})
	id := body[proposalIDKey].(string)
	_, err := s.GetProposal(context.Background(), "other", "chief", id)
	require.ErrorIs(t, err, models.ErrProposalNotFound)
	_, err = s.GetProposal(context.Background(), "ws", "chief", "missing")
	require.ErrorIs(t, err, models.ErrProposalNotFound)
	rows, err := s.ListProposals(context.Background(), "ws", "chief", models.ProposalPending, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	_, err = s.ListProposals(context.Background(), "other", "chief", "", 10)
	require.ErrorIs(t, err, models.ErrProposalNotFound)
}

func TestPromptRendersApprovedDecisions(t *testing.T) {
	var text strings.Builder
	writeProposalDecisions(&text, []proposalDecisionUpdate{
		{ProposalID: "p1", Outcome: "approved", Title: "One", TaskID: "t1", Duplicate: true},
		{ProposalID: "p2", Outcome: "edited", Title: "Two", TaskID: "t2"},
	})
	require.Contains(t, text.String(), "Task proposal decisions (made by the user in chat):")
	require.Contains(t, text.String(), `- "One" (proposal_id=p1): approved; created task_id=t1 [already existed]`)
	require.Contains(t, text.String(), `- "Two" (proposal_id=p2): approved with the user's edits; created task_id=t2`)
}
