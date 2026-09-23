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
}

func (s *Service) QueueTurn(ctx context.Context, id, taskID, reason, key string, payload map[string]any) error {
	if err := s.CheckConversationExecution(ctx, taskID); err != nil {
		return err
	}
	a, err := s.Personas.GetAgentInstance(ctx, id)
	if err != nil {
		return err
	}
	if paused(a) {
		return fmt.Errorf("coordinator is paused")
	}
	role, err := s.Repo.OrchestratorRoleID(ctx, id)
	if err != nil {
		return err
	}
	if role == "" {
		return fmt.Errorf("coordinator not registered")
	}
	payload, err = s.withIntentRevision(ctx, taskID, payload)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Queue == nil {
		return fmt.Errorf("orchestration queue is not ready")
	}
	_, err = s.Queue.QueueRun(ctx, runservice.QueueRunRequest{AgentProfileID: id, TaskID: taskID, Reason: reason, IdempotencyKey: key, Payload: payload, DisableCoalescing: true})
	return err
}

// Process claims no rows itself: the core queue dispatcher owns the single claim loop.
func (s *Service) Process(ctx context.Context, run *runmodels.Run) (bool, error) {
	role, err := s.Repo.OrchestratorRoleID(ctx, run.AgentProfileID)
	if err != nil || role == "" {
		return false, err
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
func (s *Service) prompt(ctx context.Context, a *models.AgentInstance, taskID string, payload map[string]any) (string, error) {
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
		fmt.Fprintf(&text, "\nCurrent user message (comment_id=%s, intent_revision=%v): %s\n", comment.ID, payload[intentRevisionKey], comment.Body)
	}
	writeTaskUpdates(&text, updatesForPrompt(payload))
	return text.String(), nil
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
