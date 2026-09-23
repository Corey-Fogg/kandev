package runtime

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantContextScopesBudgetAndMandatoryOverflow(t *testing.T) {
	b := &models.AssistantBinding{ID: "binding", OwnerUserID: "owner", WorkspaceID: "ws"}
	p := &models.ContextPacket{ContextScope: models.ContextScope{ProfileID: "profile", TaskID: "task"}}
	expired := time.Now().Add(-time.Hour)
	rows := []*models.AgentMemory{
		{ID: "global-legacy", Scope: "user", Content: "UNOWNED_GLOBAL"},
		{ID: "foreign", Scope: "workspace", ScopeID: "other", Content: "FOREIGN"},
		{ID: "expired", Scope: "workspace", ExpiresAt: &expired, Content: "EXPIRED"},
		{ID: "project", Scope: "project", ScopeID: "other", Content: "PROJECT"},
		{ID: "match", Scope: "task", ScopeID: "task", Content: strings.Repeat("界", 1000)},
	}
	require.NoError(t, fillContextMemory(p, b, rows))
	require.Len(t, p.Memory, 1)
	require.True(t, p.Memory[0].Truncated)
	require.True(t, utf8.ValidString(p.Memory[0].Content))
	require.LessOrEqual(t, len(p.Memory[0].Content), 1024)
	required := &models.AgentMemory{ID: "must", Scope: "workspace", Confirmed: true, Priority: 100, Content: strings.Repeat("a", 1025)}
	require.ErrorContains(t, fillContextMemory(&models.ContextPacket{}, b, []*models.AgentMemory{required}), "required memory")
	p = &models.ContextPacket{UserInstruction: strings.Repeat("x", models.ContextBudgetBytes)}
	require.ErrorContains(t, fillContextMemory(p, b, nil), "indispensable")
}
