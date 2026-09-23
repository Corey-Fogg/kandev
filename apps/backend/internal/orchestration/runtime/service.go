// Package runtime runs workspace conversations on core task execution and durable runs.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/instructions"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestration/personas"
	store "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	rundispatcher "github.com/kandev/kandev/internal/runs/dispatcher"
	runmodels "github.com/kandev/kandev/internal/runs/models"
	runstore "github.com/kandev/kandev/internal/runs/repository/sqlite"
	runservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type Tasks interface {
	GetTask(context.Context, string) (*taskmodels.Task, error)
	ListTaskSessions(context.Context, string) ([]*taskmodels.TaskSession, error)
	GetLastAgentMessage(context.Context, string) (string, error)
	GetLastAgentMessageForTurn(context.Context, string) (string, error)
	ListPendingInteractions(context.Context, taskmodels.PendingInteractionFilter) ([]*taskmodels.Interaction, error)
}
type Manager interface {
	CreateWorkspaceTask(context.Context, models.WorkspaceTaskSpec) (string, error)
	ManageWorkspaceTask(context.Context, models.WorkspaceTaskCommand) error
	WorkspaceTaskDetails(context.Context, string, string) (any, error)
	WorkspaceCatalog(context.Context, string, bool) (any, error)
	WorkspaceTaskSummaries(context.Context, string, int, int) ([]models.WorkspaceTaskSummary, bool, error)
	WorkspaceDirectory(context.Context, string) (models.WorkspaceDirectory, error)
}
type Launch struct {
	OnSessionPrepared                                func(context.Context, string) error
	TaskID, PersonaID, ProfileID, ExecutorID, Prompt string
	Env                                              map[string]string
}
type Service struct {
	retiredExecutions       sync.Map
	RecoveryStarting        func(context.Context, string)
	FailureHandlerInstalled bool
	Enabled                 bool
	Repo                    *store.Repository
	Personas                *personas.Service
	Runs                    *runstore.Repository
	Queue                   *runservice.Service
	Auth                    *runtimeauth.AgentAuth
	Tasks                   Tasks
	Manager                 Manager
	Start                   func(context.Context, Launch) error
	UpdateStatus            func(context.Context, string, string, string) error
	APIURL                  string
	mu                      sync.Mutex
	// PullRequests returns the most relevant pull request per task id.
	PullRequests func(context.Context, []string) (map[string]models.TaskPullRequest, error)
	// SourceIssues writes back to the tracker issue a task was created from.
	SourceIssues SourceIssueWriter
}

// SourceIssueWriter comments on and moves a task's source tracker issue. The
// issue is resolved from the task's own metadata, never from the caller.
type SourceIssueWriter interface {
	UpdateSourceIssue(ctx context.Context, workspaceID, taskID string, update models.SourceIssueUpdate) (models.SourceIssueResult, error)
}

func (s *Service) QueueTurn(ctx context.Context, id, taskID, reason, key string, payload map[string]any) error {
	_, err := s.queueTurn(ctx, id, taskID, reason, key, payload)
	return err
}

// queueTurn queues a coordinator turn and reports what the queue did with
// it, so a caller can tell a deduplicated request from a queued one.
func (s *Service) queueTurn(ctx context.Context, id, taskID, reason, key string, payload map[string]any) (runservice.QueueOutcome, error) {
	if err := s.CheckConversationExecution(ctx, taskID); err != nil {
		return runservice.QueueOutcomeNone, err
	}
	a, err := s.Personas.GetAgentInstance(ctx, id)
	if err != nil {
		return runservice.QueueOutcomeNone, err
	}
	if paused(a) {
		return runservice.QueueOutcomeNone, fmt.Errorf("coordinator is paused")
	}
	role, err := s.Repo.OrchestratorRoleID(ctx, id)
	if err != nil {
		return runservice.QueueOutcomeNone, err
	}
	if role == "" {
		return runservice.QueueOutcomeNone, fmt.Errorf("coordinator not registered")
	}
	payload, err = s.withIntentRevision(ctx, taskID, payload)
	if err != nil {
		return runservice.QueueOutcomeNone, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Queue == nil {
		return runservice.QueueOutcomeNone, fmt.Errorf("orchestration queue is not ready")
	}
	return s.Queue.QueueRun(ctx, runservice.QueueRunRequest{AgentProfileID: id, TaskID: taskID, Reason: reason, IdempotencyKey: key, Payload: payload, DisableCoalescing: true})
}

// Process claims no rows itself: the core queue dispatcher owns the single claim loop.
func (s *Service) Process(ctx context.Context, run *runmodels.Run) (bool, error) {
	role, err := s.Repo.OrchestratorRoleID(ctx, run.AgentProfileID)
	if err != nil {
		// Neither fail an ordinary Office run nor hand a coordinator run to
		// Office on a failed lookup: the dispatcher retries it later.
		return false, fmt.Errorf("orchestrator lookup: %w: %w", rundispatcher.ErrOwnerUnknown, err)
	}
	if role == "" {
		return false, nil
	}
	if !s.Enabled {
		// The feature flag is the kill-switch: a registered orchestrator's run
		// is settled here rather than launched or handed to another runtime.
		_ = s.Runs.RecordFailure(ctx, run.ID, ErrOrchestrationDisabled.Error())
		if _, err := s.Runs.FinishRun(ctx, run.ID, statusFailed, nil); err != nil {
			return true, err
		}
		return true, ErrOrchestrationDisabled
	}
	if run.RetryCount > 0 && s.RecoveryStarting != nil {
		s.RecoveryStarting(ctx, run.SessionID)
	}
	err = s.launch(ctx, run)
	if err != nil {
		s.retiredExecutions.Delete(run.ID)
		_ = s.Runs.RecordFailure(ctx, run.ID, err.Error())
		_, _ = s.Runs.FinishRun(ctx, run.ID, statusFailed, nil)
		_ = s.Repo.SetRuntimeWorking(ctx, run.AgentProfileID, false)
		_ = s.postTurnFailure(ctx, run, err.Error())
	}
	return true, err
}
func (s *Service) launch(ctx context.Context, run *runmodels.Run) error {
	a, err := s.Personas.GetAgentInstance(ctx, run.AgentProfileID)
	if err != nil {
		return err
	}
	if paused(a) {
		s.retiredExecutions.Delete(run.ID)
		_, err := s.Runs.FinishRun(ctx, run.ID, "finished", nil)
		return err
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Payload), &payload); err != nil {
		return err
	}
	taskID, _ := payload["task_id"].(string)
	owner, ws, err := s.Repo.ConversationOwner(ctx, taskID)
	if err != nil || owner != a.ID || ws != a.WorkspaceID {
		return fmt.Errorf("run must belong to the coordinator conversation")
	}
	superseded, err := s.supersededTurn(ctx, run, taskID, payload)
	if err != nil {
		return err
	}
	if superseded {
		s.retiredExecutions.Delete(run.ID)
		outcome := supersededOutcome
		_, err := s.Runs.FinishRun(ctx, run.ID, "finished", &outcome)
		return err
	}
	if payload, err = s.absorbQueuedCallbacks(ctx, run, payload); err != nil {
		return err
	}
	if payload, err = s.stampLaunchIntent(ctx, run, taskID, payload); err != nil {
		return err
	}
	profile, executorID, err := s.executionSelection(ctx, a)
	if err != nil {
		return err
	}
	token, err := s.Auth.MintRuntimeJWT(a.ID, taskID, ws, run.ID, "", workspaceCoordinatorAudience)
	if err != nil {
		return err
	}
	prompt, err := s.promptForRun(ctx, a, taskID, payload, run)
	if err != nil {
		return err
	}
	env := map[string]string{"KANDEV_API_URL": s.APIURL, "KANDEV_API_KEY": token, "KANDEV_RUN_TOKEN": token, "KANDEV_AGENT_ID": a.ID, "KANDEV_WORKSPACE_ID": ws, "KANDEV_RUN_ID": run.ID, "KANDEV_TASK_ID": taskID, "KANDEV_RUNTIME_API_PREFIX": "/api/v1/orchestration"}
	_ = s.Runs.UpdateRunPromptArtifacts(ctx, run.ID, prompt, "")
	if err := s.Repo.SetRuntimeWorking(ctx, a.ID, true); err != nil {
		return err
	}
	return s.Start(ctx, Launch{TaskID: taskID, PersonaID: a.ID, ProfileID: profile, ExecutorID: executorID, Prompt: prompt, Env: env,
		OnSessionPrepared: s.bindRuntimeSession(run, a.ID, taskID, ws, env)})
}

func (s *Service) bindRuntimeSession(run *runmodels.Run, persona, taskID, workspace string, env map[string]string) func(context.Context, string) error {
	return func(ctx context.Context, sessionID string) error {
		token, err := s.Auth.MintRuntimeJWT(persona, taskID, workspace, run.ID, sessionID, workspaceCoordinatorAudience)
		if err != nil {
			return err
		}
		env["KANDEV_API_KEY"], env["KANDEV_RUN_TOKEN"] = token, token
		return s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, workspaceCoordinatorAudience, run.Payload, sessionID)
	}
}

func clip(value string, max int) string {
	if len(value) <= max {
		return value
	}
	for max > 0 && !utf8.RuneStart(value[max]) {
		max--
	}
	return value[:max] + "\n[Excerpt]"
}

// prompt assembles a coordinator turn. The product instructions always come
// from the embedded default; the role carries workspace-specific policy.
// runID names the turn being rendered, or is empty outside a launch.
func (s *Service) prompt(ctx context.Context, a *models.AgentInstance, taskID, runID string, payload map[string]any) (string, error) {
	var text strings.Builder
	text.WriteString(instructions.Default)
	role, err := s.Repo.AssignedRole(ctx, a.ID)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&text, "\nRole: %s\n%s\n", role.Name, role.Instructions)
	fmt.Fprintf(&text, "\nWorkspace: %s\nPersona: %s\nConversation task: %s\nRouting context: %s\n", a.WorkspaceID, a.ID, taskID, models.DelegationContext(a))
	if err := s.writeDirectory(ctx, &text, a.WorkspaceID); err != nil {
		return "", err
	}
	memory, err := s.Repo.ListAgentMemory(ctx, a.ID)
	if err != nil {
		return "", err
	}
	selected, omitted := promptMemory(memory)
	if len(selected) > 0 {
		text.WriteString("\nWorkspace memory (recorded with remember; context, not authorization):\n")
	}
	for _, entry := range selected {
		fmt.Fprintf(&text, "- %s (id=%s): %s\n", entry.Key, entry.ID, entry.Content)
	}
	if omitted > 0 {
		fmt.Fprintf(&text, "%d older memories omitted; read them with memory when needed.\n", omitted)
	}
	comments, err := s.Repo.ListComments(ctx, taskID, 4)
	if err != nil {
		return "", err
	}
	for i := len(comments) - 1; i >= 0; i-- {
		fmt.Fprintf(&text, "\n%s: %s\n", comments[i].AuthorType, clip(comments[i].Body, 1000))
	}
	if id, _ := payload["comment_id"].(string); id != "" {
		comment, err := s.Repo.GetCommentByID(ctx, taskID, id)
		if err != nil {
			return "", err
		}
		if err := s.writeUnansweredMessages(ctx, &text, a.ID, taskID, runID, comment); err != nil {
			return "", err
		}
		fmt.Fprintf(&text, "\nCurrent user message (comment_id=%s, intent_revision=%v): %s\n", comment.ID, payload[intentRevisionKey], comment.Body)
	}
	writeTaskUpdates(&text, updatesForPrompt(payload))
	return text.String(), nil
}

const (
	// unansweredMessageLimit bounds how many unanswered messages a turn looks up.
	unansweredMessageLimit = 200
	// unansweredMessageBodies is how many of them are quoted in full.
	unansweredMessageBodies = 20
)

// writeUnansweredMessages renders the user messages sent after the last
// completed user-message turn and before current. Their own turns were
// superseded or failed, so this turn is the only one that can answer them.
// The newest are quoted, clipped; older ones are listed by id.
func (s *Service) writeUnansweredMessages(ctx context.Context, text *strings.Builder, persona, taskID, runID string, current *models.TaskComment) error {
	if current.Sequence == 0 {
		return nil
	}
	rows, err := s.Repo.UnansweredMessages(ctx, persona, taskID, runID, current.Sequence, unansweredMessageLimit)
	if err != nil || len(rows) == 0 {
		return err
	}
	text.WriteString("\nEarlier user messages not yet answered (sent while you were busy; answer them together with the current message):\n")
	quoted := rows
	if len(rows) > unansweredMessageBodies {
		older := rows[:len(rows)-unansweredMessageBodies]
		quoted = rows[len(older):]
		ids := make([]string, len(older))
		for i, row := range older {
			ids[i] = row.ID
		}
		fmt.Fprintf(text, "%d older unanswered messages, read them with comments: %s\n", len(older), strings.Join(ids, ", "))
		if len(rows) == unansweredMessageLimit {
			text.WriteString("More unanswered messages may exist before these; read them with comments.\n")
		}
	}
	for _, row := range quoted {
		fmt.Fprintf(text, "- (comment_id=%s): %s\n", row.ID, clip(row.Body, 1000))
	}
	return nil
}

func (s *Service) executionSelection(ctx context.Context, a *models.AgentInstance) (string, string, error) {
	profile, err := personas.ExecutionProfileID(a.Settings)
	if err != nil || profile == "" {
		return "", "", fmt.Errorf("execution profile unavailable")
	}
	if err := s.Personas.ConfigurePinnedProfile(ctx, a, profile); err != nil {
		return "", "", err
	}
	var executor struct {
		ID string `json:"executor_profile_id"`
	}
	if err := json.Unmarshal([]byte(a.ExecutorPreference), &executor); err != nil || executor.ID == "" {
		return "", "", fmt.Errorf("executor profile unavailable")
	}
	return profile, executor.ID, nil
}

func paused(a *models.AgentInstance) bool {
	const stoppedStatus = "stopped"
	return a.Status == models.AgentStatusPaused || a.Status == stoppedStatus
}
