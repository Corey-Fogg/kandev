import { useCallback, useEffect, useRef, useState } from "react";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import {
  getOrchestratorMetrics,
  type CoordinatorMetrics,
} from "@/lib/api/domains/orchestration-api";

type Result = { key: string; data?: CoordinatorMetrics; error?: unknown };

/**
 * Reads one orchestrator's outcome metrics for a 7 or 30 day window. Only the
 * latest request may commit, and a result is shown only for the scope that
 * requested it, so switching orchestrator or window never shows stale numbers.
 */
export function useCoordinatorMetrics(workspaceId: string, orchestratorId: string, days: 7 | 30) {
  const key = orchestratorId ? JSON.stringify([workspaceId, orchestratorId, days]) : "";
  const [result, setResult] = useState<Result>({ key: "" });
  const [revision, setRevision] = useState(0);
  const generation = useRef(0);
  useEffect(() => {
    const current = ++generation.current;
    if (!orchestratorId) return;
    const controller = new AbortController();
    getOrchestratorMetrics(workspaceId, orchestratorId, days, { signal: controller.signal })
      .then((data) => {
        if (current === generation.current) setResult({ key, data });
      })
      .catch((error: unknown) => {
        if (current === generation.current && !controller.signal.aborted) setResult({ key, error });
      });
    return () => controller.abort();
  }, [workspaceId, orchestratorId, days, key, revision]);
  const refresh = useCallback(() => setRevision((value) => value + 1), []);
  useForegroundRefresh(refresh, !!orchestratorId, key);
  const current = result.key === key ? result : undefined;
  return {
    data: current?.data,
    error: current?.error,
    loading: !!key && !current,
    refresh,
  };
}
