import { useCallback, useState } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { toast } from "@/lib/toast/sonner";
import {
  openOrchestratorConversation,
  orchestratorConversationHref,
} from "@/lib/api/domains/orchestration-api";
import { readWorkspaceOrchestrators } from "@/lib/orchestration/orchestrator-list-cache";
import { useOrchestrationData } from "./use-orchestration-data";

/** The workspace's orchestrators; an empty workspace id reads nothing. */
export function useWorkspaceOrchestrators(workspaceId: string) {
  const load = useCallback(
    () =>
      workspaceId
        ? readWorkspaceOrchestrators(workspaceId)
        : Promise.resolve({ orchestrators: [] }),
    [workspaceId],
  );
  return useOrchestrationData(load, undefined, workspaceId);
}

export function useOrchestratorConversation(
  workspaceId: string,
  id: string,
  onNavigate?: () => void,
) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const open = async () => {
    if (busy) return;
    setBusy(true);
    try {
      const conversation = await openOrchestratorConversation(workspaceId, id);
      router.push(orchestratorConversationHref(workspaceId, id, conversation.task_id));
      onNavigate?.();
    } catch (error) {
      toast.error(String(error));
    } finally {
      setBusy(false);
    }
  };
  return { open, busy };
}
