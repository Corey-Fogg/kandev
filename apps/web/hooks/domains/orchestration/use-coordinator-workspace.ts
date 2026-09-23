import { useCallback, useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { fetchJson } from "@/lib/api/client";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { listAllExecutorProfiles } from "@/lib/api/domains/settings-api";
import { listOrchestrationProfiles } from "@/lib/api/domains/orchestration-api";
import { readWorkspaceOrchestrators } from "@/lib/orchestration/orchestrator-list-cache";
import type { Workspace, ListWorkflowStepsResponse } from "@/lib/types/http";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { useOrchestrationData } from "./use-orchestration-data";

type CoordinatorWorkspaceIdentity = {
  id: string;
  name: string;
  office_workflow_id?: string | null;
};

function useStableReads(workspaceId: string) {
  const store = useAppStoreApi();
  const executors = useRef<ReturnType<typeof listAllExecutorProfiles>>(undefined);
  return useCallback(() => {
    const known = store.getState().workspaces.items.find((item) => item.id === workspaceId);
    if (!executors.current) {
      const request = listAllExecutorProfiles();
      executors.current = request;
      request.catch(() => {
        if (executors.current === request) executors.current = undefined;
      });
    }
    const workspace: Promise<CoordinatorWorkspaceIdentity> = known
      ? Promise.resolve(known)
      : fetchJson<Workspace>(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}`);
    return { workspace, executors: executors.current };
  }, [store, workspaceId]);
}

/**
 * Reads what the coordinator page needs for one workspace. The workspace
 * identity comes from the store when it is known, and executor profiles are
 * read once per page; foreground refreshes re-read only the workspace data.
 */
export function useCoordinatorWorkspace(workspaceId: string) {
  const owner = useAppStore((s) => s.auth.user?.id);
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);
  const stableReads = useStableReads(workspaceId);
  const load = useCallback(async () => {
    const stable = stableReads();
    const [workspace, workflows, steps, repositories, assignments, profiles, executors] =
      await Promise.all([
        stable.workspace,
        listWorkflows(workspaceId),
        fetchJson<ListWorkflowStepsResponse>(
          `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/workflow-steps`,
        ),
        listRepositories(workspaceId),
        readWorkspaceOrchestrators(workspaceId),
        listOrchestrationProfiles(workspaceId),
        stable.executors,
      ]);
    return {
      workspace,
      workflows: workflows.workflows,
      steps: steps.steps,
      repositories: repositories.repositories,
      assignments: assignments.orchestrators,
      profiles: profiles.profiles,
      executors: executors.profiles,
    };
  }, [stableReads, workspaceId]);
  const result = useOrchestrationData(load, owner, workspaceId);
  useEffect(() => {
    if (result.data?.workspace.id === workspaceId) setActiveWorkspace(workspaceId);
  }, [result.data, workspaceId, setActiveWorkspace]);
  useForegroundRefresh(result.refresh, true, workspaceId);
  return result;
}
export type CoordinatorWorkspace = NonNullable<ReturnType<typeof useCoordinatorWorkspace>["data"]>;
