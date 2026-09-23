import { useAppStore } from "@/components/state-provider";
import { useCallback } from "react";
import { openOrchestratorConversation } from "@/lib/api/domains/orchestration-api";
import { useOrchestrationData } from "./use-orchestration-data";

type Conversation = Awaited<ReturnType<typeof openOrchestratorConversation>>;
const conversations = new Map<string, Promise<Conversation>>();

/** An assignment's conversation task is stable, so it is opened once per viewer. */
function openCoordinatorConversation(
  owner: string | undefined,
  workspaceId: string,
  orchestratorId: string,
): Promise<Conversation> {
  const key = JSON.stringify([owner ?? "", workspaceId, orchestratorId]);
  const cached = conversations.get(key);
  if (cached) return cached;
  const request = openOrchestratorConversation(workspaceId, orchestratorId);
  conversations.set(key, request);
  request.catch(() => {
    if (conversations.get(key) === request) conversations.delete(key);
  });
  return request;
}

export function useCoordinatorConversation(workspaceId: string, orchestratorId: string) {
  const owner = useAppStore((s) => s.auth.user?.id);
  const load = useCallback(
    () => openCoordinatorConversation(owner, workspaceId, orchestratorId),
    [owner, workspaceId, orchestratorId],
  );
  return useOrchestrationData(load, owner, workspaceId);
}
