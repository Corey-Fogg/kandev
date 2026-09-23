import type { AppState } from "@/lib/state/app-state-types";

type StoreTask = AppState["kanban"]["tasks"][number];

/**
 * Counts the loaded coordinator-managed tasks of the active workspace that are
 * waiting on a question or permission answer. It reads the board's live task
 * projection, so it adds no request of its own. With an orchestrator id it counts
 * only the tasks that orchestrator delegated.
 */
export function selectCoordinatedPendingInputCount(
  state: AppState,
  orchestratorId?: string,
): number {
  const waiting = new Set<string>();
  const visit = (tasks: StoreTask[]) => {
    for (const task of tasks) {
      const coordinator = task.metadata?.orchestration_chief_id;
      if (typeof coordinator !== "string" || !coordinator) continue;
      if (orchestratorId !== undefined && coordinator !== orchestratorId) continue;
      if (task.statusSummary?.pending_action) waiting.add(task.id);
    }
  };
  visit(state.kanban.tasks);
  for (const snapshot of Object.values(state.kanbanMulti.snapshots)) visit(snapshot.tasks);
  return waiting.size;
}
