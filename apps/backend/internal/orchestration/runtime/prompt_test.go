package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestration/instructions"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func TestPromptUsesEmbeddedInstructionsAndWorkspaceDirectory(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	require.NoError(t, s.Repo.UpsertInstruction(ctx, "chief", "AGENTS.md", "Use `kandev task create` from the shell.", true))
	s.Manager = &fakeTaskManager{directory: models.WorkspaceDirectory{
		Workflows:    []models.DirectoryWorkflow{{ID: "wf", Name: "Delivery", Steps: []models.DirectoryStep{{ID: "todo", Name: "Todo", Start: true}, {ID: "review", Name: "Review"}}}},
		Repositories: []models.DirectoryRepository{{ID: "repo", Name: "Example repository", DefaultBranch: "main"}},
	}}
	persona, err := s.Personas.GetAgentInstance(ctx, "chief")
	require.NoError(t, err)
	prompt, err := s.prompt(ctx, persona, task, "", nil)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(prompt, instructions.Default), "the embedded instructions open every turn")
	require.NotContains(t, prompt, "kandev task create", "a stored instruction copy is never injected")
	require.Contains(t, prompt, "- workflow Delivery (id=wf): Todo (id=todo, start); Review (id=review)\n")
	require.Contains(t, prompt, "- repository Example repository (id=repo, branch main)\n")
	require.Contains(t, prompt, "- execution profile Personal (id=personal, agent claude)\n")
}

func TestPromptDirectoryIsBounded(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	manager := &fakeTaskManager{}
	for i := 0; i < 200; i++ {
		manager.directory.Repositories = append(manager.directory.Repositories, models.DirectoryRepository{ID: strings.Repeat("r", 36), Name: "Example repository"})
	}
	s.Manager = manager
	persona, err := s.Personas.GetAgentInstance(ctx, "chief")
	require.NoError(t, err)
	prompt, err := s.prompt(ctx, persona, task, "", nil)
	require.NoError(t, err)
	require.Contains(t, prompt, "directory truncated; read workspace for the rest")
	start := strings.Index(prompt, "Workspace directory")
	end := strings.Index(prompt, "directory truncated")
	require.Less(t, end-start, directoryPromptBytes+100)
}
