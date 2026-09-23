import { useCallback, useEffect, useState } from "react";
import { invalidateWorkspaceOrchestrators } from "@/lib/orchestration/orchestrator-list-cache";

export const ORCHESTRATION_CHANGED = "kandev:orchestration-changed";
type OrchestrationChange = { workspaceId?: string };

/** Announces an orchestration write; omit the workspace for global changes such as roles. */
export const notifyOrchestrationChanged = (workspaceId?: string) => {
  invalidateWorkspaceOrchestrators(workspaceId);
  window.dispatchEvent(
    new CustomEvent<OrchestrationChange>(ORCHESTRATION_CHANGED, { detail: { workspaceId } }),
  );
};

/** True when a change event concerns `workspaceId` (or the listener is not workspace scoped). */
export function affectsWorkspace(event: Event, workspaceId?: string) {
  const changed = (event as CustomEvent<OrchestrationChange>).detail?.workspaceId;
  return !workspaceId || !changed || changed === workspaceId;
}

export function useOrchestrationData<T>(
  load: () => Promise<T>,
  scopeKey?: string,
  workspaceId?: string,
) {
  const [snapshot, setSnapshot] = useState<{ load: typeof load; scopeKey?: string; data: T }>();
  const [error, setError] = useState<{ load: typeof load; scopeKey?: string; message: string }>();
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true;
    setError(undefined);

    void load()
      .then((v) => {
        if (active) setSnapshot({ load, scopeKey, data: v });
      })
      .catch((e) => {
        if (active) setError({ load, scopeKey, message: String(e.message ?? e) });
      });
    return () => {
      active = false;
    };
  }, [load, revision, scopeKey]);
  useEffect(() => {
    const changed = (event: Event) => {
      if (affectsWorkspace(event, workspaceId)) setRevision((v) => v + 1);
    };
    window.addEventListener(ORCHESTRATION_CHANGED, changed);
    return () => window.removeEventListener(ORCHESTRATION_CHANGED, changed);
  }, [workspaceId]);
  const refresh = useCallback(() => setRevision((value) => value + 1), []);
  return {
    data: snapshot?.load === load && snapshot.scopeKey === scopeKey ? snapshot.data : undefined,
    error: error?.load === load && error.scopeKey === scopeKey ? error.message : undefined,
    refresh,
  };
}
