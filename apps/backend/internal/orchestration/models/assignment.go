package models

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// OrchestratorSettings are the per-orchestrator behavior switches.
type OrchestratorSettings struct {
	AskBeforeCreate    bool `json:"ask_before_create" db:"ask_before_create"`
	AutoCommentSource  bool `json:"auto_comment_source" db:"auto_comment_source"`
	AutoMoveSourceDone bool `json:"auto_move_source_done" db:"auto_move_source_done"`
}

// OrchestratorAssignment is one workspace orchestrator registration: the
// role template it follows, its own instance name and its settings.
type OrchestratorAssignment struct {
	AgentID     string `db:"agent_id"`
	WorkspaceID string `db:"workspace_id"`
	RoleID      string `db:"role_id"`
	// DisplayName is the instance name; empty inherits the role name.
	DisplayName string `db:"display_name"`
	OrchestratorSettings
}

// OrchestratorPatch changes some of an orchestrator's name and settings. A
// nil field is left unchanged.
type OrchestratorPatch struct {
	DisplayName        *string `json:"display_name"`
	AskBeforeCreate    *bool   `json:"ask_before_create"`
	AutoCommentSource  *bool   `json:"auto_comment_source"`
	AutoMoveSourceDone *bool   `json:"auto_move_source_done"`
}

// Empty reports whether the patch changes nothing.
func (p OrchestratorPatch) Empty() bool {
	return p.DisplayName == nil && p.AskBeforeCreate == nil && p.AutoCommentSource == nil && p.AutoMoveSourceDone == nil
}

// ApplySettings overlays the patch's settings on s.
func (p OrchestratorPatch) ApplySettings(s OrchestratorSettings) OrchestratorSettings {
	if p.AskBeforeCreate != nil {
		s.AskBeforeCreate = *p.AskBeforeCreate
	}
	if p.AutoCommentSource != nil {
		s.AutoCommentSource = *p.AutoCommentSource
	}
	if p.AutoMoveSourceDone != nil {
		s.AutoMoveSourceDone = *p.AutoMoveSourceDone
	}
	return s
}

// DisplayNameMaxRunes bounds an orchestrator's instance name.
const DisplayNameMaxRunes = 60

// ErrInvalidDisplayName means an instance name is too long or not plain text.
var ErrInvalidDisplayName = errors.New("display_name must be at most 60 characters of text without control characters")

// NormalizeDisplayName trims an instance name and validates it. An empty
// result means "use the role name".
func NormalizeDisplayName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > DisplayNameMaxRunes {
		return "", ErrInvalidDisplayName
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrInvalidDisplayName
		}
	}
	return name, nil
}

// EffectiveName is the name an orchestrator goes by: its instance name, or
// its role name when it has none.
func EffectiveName(displayName, roleName string) string {
	if displayName != "" {
		return displayName
	}
	return roleName
}

// DefaultOrchestratorSettings are the settings of a new orchestrator:
// create tasks directly, comment on source issues, never move them.
func DefaultOrchestratorSettings() OrchestratorSettings {
	return OrchestratorSettings{AskBeforeCreate: false, AutoCommentSource: true, AutoMoveSourceDone: false}
}
