package runtime

import (
	"context"
	"fmt"
	"strings"
)

// directoryPromptBytes bounds the workspace directory rendered into a turn.
const directoryPromptBytes = 3000

// writeDirectory renders the IDs a coordinator selects from when it creates,
// assigns or moves tasks, so an ordinary delegation needs no workspace read.
func (s *Service) writeDirectory(ctx context.Context, text *strings.Builder, workspaceID string) error {
	var body strings.Builder
	if s.Manager != nil {
		directory, err := s.Manager.WorkspaceDirectory(ctx, workspaceID)
		if err != nil {
			return err
		}
		for _, workflow := range directory.Workflows {
			fmt.Fprintf(&body, "- workflow %s (id=%s):", workflow.Name, workflow.ID)
			for i, step := range workflow.Steps {
				if i > 0 {
					body.WriteString(";")
				}
				fmt.Fprintf(&body, " %s (id=%s", step.Name, step.ID)
				if step.Start {
					body.WriteString(", start")
				}
				body.WriteString(")")
			}
			body.WriteString("\n")
		}
		for _, repository := range directory.Repositories {
			fmt.Fprintf(&body, "- repository %s (id=%s", repository.Name, repository.ID)
			if repository.DefaultBranch != "" {
				fmt.Fprintf(&body, ", branch %s", repository.DefaultBranch)
			}
			body.WriteString(")\n")
		}
	}
	profiles, err := s.Repo.ExecutionProfileDirectory(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		fmt.Fprintf(&body, "- execution profile %s (id=%s, agent %s)\n", profile["name"], profile["id"], profile["agent_id"])
	}
	if body.Len() == 0 {
		return nil
	}
	text.WriteString("\nWorkspace directory (read workspace for full configuration):\n")
	rendered := body.String()
	if len(rendered) > directoryPromptBytes {
		cut := strings.LastIndexByte(rendered[:directoryPromptBytes], '\n')
		rendered = rendered[:cut+1] + "- directory truncated; read workspace for the rest\n"
	}
	text.WriteString(rendered)
	return nil
}
