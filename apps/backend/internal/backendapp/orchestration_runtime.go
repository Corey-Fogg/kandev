package backendapp

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/ports"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestration/personas"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	"github.com/kandev/kandev/internal/orchestrator"
	orchexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/workflow/stepevents"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func newOrchestrationRuntime(cfg *config.Config, repos *Repositories, services *Services, orch *orchestrator.Service, eventBus bus.EventBus, log *logger.Logger) *orchestrationruntime.Service {
	apiPort := cfg.Server.Port
	if apiPort == 0 {
		apiPort = ports.Backend
	}
	personasSvc := &personas.Service{Profiles: repos.AgentSettings, Repo: repos.Orchestration, Terminate: func(ctx context.Context, id string) error {
		return orch.PersonaSessionTerminator().TerminateAllForAgent(ctx, id, "orchestrator_deleted")
	}}
	runtime := &orchestrationruntime.Service{
		FailureHandlerInstalled: true,
		RecoveryStarting:        orch.ResolveManagedRecovery,
		Enabled:                 cfg.Features.Orchestration,
		Repo:                    repos.Orchestration, Personas: personasSvc, Runs: repos.Runs,
		Auth: runtimeauth.NewAgentAuth(""), Tasks: services.Task,
		Manager: &workspaceAdminAdapter{
			taskCreatorAdapter: &taskCreatorAdapter{taskSvc: services.Task, profiles: repos.AgentSettings, orch: orch, taskRepo: repos.Task, workflow: repos.Workflow},
			workflows:          services.Workflow, stepEvents: stepevents.NewPublisher(eventBus, "orchestration", log),
		},
		APIURL: fmt.Sprintf("http://localhost:%d", apiPort),
		Start: func(ctx context.Context, launch orchestrationruntime.Launch) error {
			_, err := orch.StartTaskWithRoute(ctx, launch.TaskID, launch.PersonaID, orchestrationLaunchContext(repos, launch), orchexecutor.RouteOverride{ExecutionProfileID: launch.ProfileID})
			return err
		},
		UpdateStatus: func(ctx context.Context, ws, id, status string) error {
			return updateOrchestratedStatus(ctx, services.Task, repos, ws, id, status)
		},
	}
	orch.SetManagedFailureRecovery(runtime.HandleFailure, runtime.CancelRecovery)
	return runtime
}
func updateOrchestratedStatus(ctx context.Context, tasks *taskservice.Service, repos *Repositories, ws, id, status string) error {
	task, err := tasks.GetTask(ctx, id)
	if err != nil || task.WorkspaceID != ws {
		return fmt.Errorf("task must belong to this workspace")
	}
	states := map[string]v1.TaskState{"done": v1.TaskStateCompleted, "COMPLETED": v1.TaskStateCompleted, "todo": v1.TaskStateTODO, "in_progress": v1.TaskStateInProgress, "in_review": v1.TaskStateReview, "review": v1.TaskStateReview}
	state, ok := states[status]
	if !ok {
		return fmt.Errorf("use done, todo, in_progress or in_review")
	}
	if state == v1.TaskStateCompleted {
		if err := validateOrchestratedCompletion(ctx, repos, task.WorkflowStepID, id); err != nil {
			return err
		}
	}
	_, err = tasks.UpdateTask(ctx, id, &taskservice.UpdateTaskRequest{State: &state})
	if err != nil {
		return err
	}
	if (state == v1.TaskStateTODO || state == v1.TaskStateInProgress) && (task.State == v1.TaskStateCompleted || task.State == v1.TaskStateReview) {
		return repos.Workflow.SupersedeTaskDecisions(ctx, id)
	}
	return nil
}

func validateOrchestratedCompletion(ctx context.Context, repos *Repositories, stepID, id string) error {
	participants, err := repos.Workflow.ListStepParticipantsForTask(ctx, stepID, id)
	if err != nil {
		return err
	}
	decisions, err := repos.Workflow.ListActiveTaskDecisions(ctx, id)
	if err != nil {
		return err
	}
	latest := map[string]string{}
	for _, decision := range decisions {
		latest[decision.ParticipantID] = strings.ToLower(decision.Decision)
	}
	for _, participant := range participants {
		if participant.DecisionRequired && latest[participant.ID] != "approved" {
			return fmt.Errorf("required review or approval is pending")
		}
	}
	return nil
}

// startOrchestrationRuntime attaches the conversation runtime to the event bus
// and its dependents before run dispatch starts. Interrupted conversation runs
// are settled first so a restart never leaves a turn claimed forever. The
// coordinator dispatch guard is installed whether or not the feature is on,
// so a coordinator conversation never dispatches outside its broker session.
func startOrchestrationRuntime(
	ctx context.Context, cfg *config.Config, services *Services, orch dispatchGuardSetter,
	repos *Repositories, eventBus bus.EventBus, addCleanup func(func() error), log *logger.Logger,
) bool {
	wireCoordinatorDispatch(orch, services.Orchestration, repos.Orchestration)
	if services.Orchestration == nil || !cfg.Features.Orchestration {
		return true
	}
	if services.Automation != nil {
		services.Automation.Service.SetOrchestratorTarget(services.Orchestration)
	}
	if err := services.Orchestration.RecoverInterrupted(ctx); err != nil {
		log.Error("orchestration recovery failed", zap.Error(err))
		return false
	}
	cleanup, err := services.Orchestration.Subscribe(eventBus)
	if err != nil {
		log.Error("orchestration subscriptions failed", zap.Error(err))
		return false
	}
	addCleanup(func() error { cleanup(); return nil })
	return true
}
