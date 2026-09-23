import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

export type TaskPRInfo = {
  number: number;
  state: string;
  aggregateState?: string;
  autoFixEnabled?: boolean;
  autoMergeEnabled?: boolean;
};

function capitalize(value: string): string {
  return value.length > 0 ? value[0].toUpperCase() + value.slice(1) : value;
}

/** Map the bounded task-level PR projection to the shared task icon shape. */
export function taskPRInfoFromSummary(
  summary: TaskStatusSummary | null | undefined,
): TaskPRInfo | undefined {
  const pullRequest = summary?.pull_request;
  if (!pullRequest?.number) return undefined;
  return {
    number: pullRequest.number,
    state: capitalize(pullRequest.state ?? pullRequest.aggregate_state ?? "open"),
    aggregateState: pullRequest.aggregate_state,
    ...(pullRequest.auto_fix_enabled ? { autoFixEnabled: true } : {}),
    ...(pullRequest.auto_merge_enabled ? { autoMergeEnabled: true } : {}),
  };
}

/**
 * The `owner/repo` of a task's pull request, read from its URL. The sidebar
 * prefers it over the task's own repository slug for the repository filter.
 */
export function repositoryPathFromSummary(
  summary: TaskStatusSummary | null | undefined,
): string | undefined {
  const url = summary?.pull_request?.url;
  if (!url) return undefined;
  try {
    const path = new URL(url).pathname.split("/").filter(Boolean);
    if (path.length >= 2) return `${path[0]}/${path[1]}`;
  } catch {
    // A malformed provider URL must not make the task switcher disappear.
  }
  return undefined;
}
