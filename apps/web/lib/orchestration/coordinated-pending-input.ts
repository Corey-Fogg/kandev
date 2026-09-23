import type { AppState } from "@/lib/state/app-state-types";

type StoreTask = AppState["kanban"]["tasks"][number];

/**
 * Counts the loaded coordinator-managed tasks of the active workspace that are
 * waiting on a question or permission answer. It reads the board's live task
 * projection, so it adds no request of its own.
 */
export function selectCoordinatedPendingInputCount(state: AppState): number {
  const waiting = new Set<string>();
  const visit = (tasks: StoreTask[]) => {
    for (const task of tasks) {
      const coordinator = task.metadata?.orchestration_chief_id;
      if (typeof coordinator === "string" && coordinator && task.statusSummary?.pending_action)
        waiting.add(task.id);
    }
  };
  visit(state.kanban.tasks);
  for (const snapshot of Object.values(state.kanbanMulti.snapshots)) visit(snapshot.tasks);
  return waiting.size;
}
