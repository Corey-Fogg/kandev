package orchestration

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestration/personas"
)

type configuration struct {
	Icon               string `json:"icon"`
	Name               string `json:"name"`
	RoleID             string `json:"role_id"`
	ProfileID          string `json:"profile_id"`
	ExecutorPreference string `json:"executor_preference"`
	Instructions       string `json:"instructions"`
	Context            string `json:"context"`
	// DisplayName and the settings below are optional in requests: nil
	// leaves the stored value unchanged.
	DisplayName        *string `json:"display_name"`
	AskBeforeCreate    *bool   `json:"ask_before_create"`
	AutoCommentSource  *bool   `json:"auto_comment_source"`
	AutoMoveSourceDone *bool   `json:"auto_move_source_done"`
}

// patch is the configuration's name and settings as a patch.
func (c *configuration) patch() models.OrchestratorPatch {
	return models.OrchestratorPatch{DisplayName: c.DisplayName, AskBeforeCreate: c.AskBeforeCreate, AutoCommentSource: c.AutoCommentSource, AutoMoveSourceDone: c.AutoMoveSourceDone}
}

func (h *Handler) describe(ctx context.Context, a *models.AgentInstance) (any, error) {
	assignment, err := h.Registry.OrchestratorAssignment(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	if assignment == nil {
		return nil, fmt.Errorf("orchestrator is not registered")
	}
	override, err := personas.ExecutionProfileID(a.Settings)
	if err != nil {
		return nil, err
	}
	definition, err := h.Registry.GetOrchestratorRole(ctx, assignment.RoleID)
	if err != nil {
		return nil, err
	}
	display, settings := assignment.DisplayName, assignment.OrchestratorSettings
	return struct {
		ID          string             `json:"id"`
		WorkspaceID string             `json:"workspace_id"`
		Status      models.AgentStatus `json:"status"`
		RoleName    string             `json:"role_name"`
		configuration
	}{a.ID, a.WorkspaceID, a.Status, definition.Name, configuration{Icon: definition.Icon, Name: models.EffectiveName(display, definition.Name), RoleID: assignment.RoleID,
		ProfileID: override, ExecutorPreference: a.ExecutorPreference, Instructions: definition.Instructions, Context: models.DelegationContext(a),
		DisplayName: &display, AskBeforeCreate: &settings.AskBeforeCreate, AutoCommentSource: &settings.AutoCommentSource, AutoMoveSourceDone: &settings.AutoMoveSourceDone}}, nil
}
func (h *Handler) prepare(c *gin.Context, a *models.AgentInstance) (*configuration, error) {
	var req configuration
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, err
	}
	if len(req.Context) > 2000 {
		return nil, fmt.Errorf("workspace context exceeds 2000 bytes")
	}
	if err := h.Repo.ValidateWorkspace(c.Request.Context(), a.WorkspaceID); err != nil {
		return nil, err
	}
	role, err := h.Registry.GetOrchestratorRole(c.Request.Context(), req.RoleID)
	if err != nil {
		return nil, fmt.Errorf("select an available role")
	}
	// Presentation and behavior are read-only projections on an assignment. Ignore
	// legacy client snapshots so workspace edits can never overwrite the global role.
	req.Name, req.Icon, req.Instructions = role.Name, role.Icon, role.Instructions
	display, err := h.preparedDisplayName(c.Request.Context(), a, req.DisplayName)
	if err != nil {
		return nil, err
	}
	profiles, err := h.Registry.ExecutionProfileDirectory(c.Request.Context(), a.WorkspaceID)
	if err != nil {
		return nil, err
	}
	valid := false
	for _, profile := range profiles {
		if profile["id"] == req.ProfileID {
			valid = true
		}
	}
	if !valid {
		return nil, fmt.Errorf("select an enabled execution profile in this workspace")
	}
	// The broker has no shell. Only a provider whose built-in tools can be
	// switched off may run a coordinator; agentctl enforces the same rule.
	agentType, err := h.Registry.ProfileAgentType(c.Request.Context(), req.ProfileID)
	if err != nil {
		return nil, err
	}
	if !mcpprofile.BrokerCapableAgent(agentType) {
		return nil, fmt.Errorf("coordinators run on Claude (claude-acp) execution profiles only; other providers keep built-in shell tools")
	}
	if h.ValidateExecutor != nil {
		if err := h.ValidateExecutor(c.Request.Context(), req.ExecutorPreference); err != nil {
			return nil, err
		}
	}
	if err := h.Agents.ConfigurePinnedProfile(c.Request.Context(), a, req.ProfileID); err != nil {
		return nil, err
	}
	a.Name = models.EffectiveName(display, role.Name)
	a.ExecutorPreference = req.ExecutorPreference
	err = setPersonaPresentation(a, req.Icon, req.Context)
	return &req, err
}
func (h *Handler) create(c *gin.Context) {
	if h.rejectExisting(c, "") {
		return
	}
	a := &models.AgentInstance{WorkspaceID: c.Param("wsId"), Role: models.AgentRoleAssistant, Status: models.AgentStatusIdle, MaxConcurrentSessions: 1}
	req, err := h.prepare(c, a)
	if err != nil {
		fail(c, err)
		return
	}
	if err = h.Agents.CreateAgentInstance(c.Request.Context(), a); err != nil {
		fail(c, err)
		return
	}
	if err = h.persistConfiguration(c.Request.Context(), a, req, true); err != nil {
		_ = h.Agents.DeleteAgentInstance(c.Request.Context(), a.ID)
		_ = h.Registry.UnregisterOrchestrator(c.Request.Context(), a.ID)
		h.failConfiguration(c, err)
		return
	}
	h.respondDescribed(c, http.StatusCreated, a.ID)
}
func (h *Handler) update(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	if a.Status == models.AgentStatusWorking {
		c.JSON(http.StatusConflict, gin.H{errorResponseKey: "wait for the current turn before changing configuration"})
		return
	}
	req, err := h.prepare(c, a)
	if err != nil {
		fail(c, err)
		return
	}
	if err = h.Agents.UpdateAgentInstance(c.Request.Context(), a); err != nil {
		fail(c, err)
		return
	}
	if err = h.persistConfiguration(c.Request.Context(), a, req, false); err != nil {
		fail(c, err)
		return
	}
	h.respondDescribed(c, http.StatusOK, a.ID)
}

// persistConfiguration stores an orchestrator's role, settings and instance
// name. create registers a new orchestrator; otherwise the existing
// registration is updated.
func (h *Handler) persistConfiguration(ctx context.Context, a *models.AgentInstance, req *configuration, create bool) error {
	settings := models.DefaultOrchestratorSettings()
	if create {
		if err := h.Registry.RegisterOrchestrator(ctx, a.ID, a.WorkspaceID, req.RoleID); err != nil {
			return err
		}
	} else {
		if err := h.Registry.UpdateOrchestratorRole(ctx, a.ID, req.RoleID); err != nil {
			return err
		}
		stored, err := h.Registry.OrchestratorAssignment(ctx, a.ID)
		if err != nil || stored == nil {
			return errors.Join(err, errors.New("orchestrator is not registered"))
		}
		settings = stored.OrchestratorSettings
	}
	if err := h.Registry.SaveOrchestratorSettings(ctx, a.ID, req.patch().ApplySettings(settings)); err != nil {
		return err
	}
	if req.DisplayName == nil || (create && *req.DisplayName == "") {
		return nil
	}
	_, err := h.Registry.SetOrchestratorDisplayName(ctx, a.ID, *req.DisplayName)
	return err
}

// preparedDisplayName validates a requested instance name, or returns the
// stored one when the request leaves it unchanged.
func (h *Handler) preparedDisplayName(ctx context.Context, a *models.AgentInstance, requested *string) (string, error) {
	if requested != nil {
		name, err := models.NormalizeDisplayName(*requested)
		if err != nil {
			return "", err
		}
		*requested = name
		return name, nil
	}
	if a.ID == "" {
		return "", nil
	}
	stored, err := h.Registry.OrchestratorAssignment(ctx, a.ID)
	if err != nil || stored == nil {
		return "", err
	}
	return stored.DisplayName, nil
}
