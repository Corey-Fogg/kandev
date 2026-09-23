import { useEffect, useMemo, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { useWebSocketClient } from "@/lib/ws/connection";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { useDebounce } from "@/hooks/use-debounce";
import {
  CoordinatorTaskObservation,
  type CoordinatorFilters,
} from "@/lib/orchestration/coordinator-task-observation";

export const SEARCH_DEBOUNCE_MS = 300;

export function useCoordinatorTasks(workspaceId: string, filters: CoordinatorFilters = {}) {
  const connection = useAppStore((s) => s.connection.status);
  const user = useAppStore((s) => s.auth.user?.id);
  const client = useWebSocketClient();
  const query = useDebounce(filters.query, SEARCH_DEBOUNCE_MS);
  const { workflowId, repositoryId } = filters;
  const observation = useMemo(
    () => new CoordinatorTaskObservation(workspaceId, {}),
    // A new viewer must never see the previous viewer's rows.
    [workspaceId, user],
  );
  const snapshot = useSyncExternalStore(
    observation.subscribe,
    observation.getSnapshot,
    observation.getSnapshot,
  );
  useEffect(() => {
    observation.activate();
    return observation.dispose;
  }, [observation]);
  useEffect(() => {
    observation.setFilters({ query, workflowId, repositoryId });
  }, [observation, query, workflowId, repositoryId]);
  useEffect(() => {
    if (connection === "connected") observation.scheduleRefresh();
  }, [connection, observation]);
  useEffect(() => {
    if (!client) return;
    const unsubscribe = [
      client.on("task.status_summary.updated", (event) => observation.applySummary(event.payload)),
      client.on("task.created", (event) => observation.lifecycle(event.payload)),
      client.on("task.updated", (event) => observation.lifecycle(event.payload)),
      client.on("task.state_changed", (event) => observation.lifecycle(event.payload)),
      client.on("task.deleted", (event) => observation.lifecycle(event.payload, true)),
    ];
    return () => unsubscribe.forEach((stop) => stop());
  }, [client, observation]);
  useForegroundRefresh(observation.refresh, true, observation);
  return { ...snapshot, refresh: observation.refresh, loadMore: observation.loadMore };
}
