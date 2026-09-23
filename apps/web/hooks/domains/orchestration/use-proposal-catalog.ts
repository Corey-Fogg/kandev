import { useCallback, useContext } from "react";
import { fetchJson } from "@/lib/api/client";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { listOrchestrationProfiles } from "@/lib/api/domains/orchestration-api";
import { ProposalCatalogContext, type ProposalCatalog } from "@/lib/orchestration/proposal-catalog";
import type { ListWorkflowStepsResponse } from "@/lib/types/http";
import { useOrchestrationData } from "./use-orchestration-data";

async function loadProposalCatalog(workspaceId: string): Promise<ProposalCatalog> {
  const [workflows, steps, repositories, profiles] = await Promise.all([
    listWorkflows(workspaceId),
    fetchJson<ListWorkflowStepsResponse>(
      `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/workflow-steps`,
    ),
    listRepositories(workspaceId),
    listOrchestrationProfiles(workspaceId),
  ]);
  return {
    workflows: workflows.workflows,
    steps: steps.steps,
    repositories: repositories.repositories,
    profiles: profiles.profiles,
  };
}

/**
 * The provided catalog, or, where no host provides one (the standalone
 * conversation route), the workspace's choices read once `enabled` is true.
 */
export function useProposalCatalog(workspaceId: string, enabled: boolean) {
  const provided = useContext(ProposalCatalogContext);
  const load = useCallback(
    () => (provided || !enabled ? Promise.resolve(provided) : loadProposalCatalog(workspaceId)),
    [provided, enabled, workspaceId],
  );
  const { data, error } = useOrchestrationData(load, undefined, workspaceId);
  return { catalog: provided ?? data ?? null, error };
}
