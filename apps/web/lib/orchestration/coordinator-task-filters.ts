import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import { DIMENSION_METAS } from "@/components/task/sidebar-filter/filter-dimension-registry";
import { applyFilters, viewRequiresArchivedTasks } from "@/lib/sidebar/apply-view";
import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { isIssueWatchFromMetadata, isPRReviewFromMetadata } from "@/lib/metadata-utils";
import { repositorySlug } from "@/lib/repository-slug";
import { repositoryPathFromSummary, taskPRInfoFromSummary } from "@/lib/task-pr-info";
import type { FilterClause } from "@/lib/state/slices/ui/sidebar-view-types";
import type { Repository, Task } from "@/lib/types/http";
import { COORDINATOR_GROUPS, type CoordinatorTaskGroup } from "./coordinator-task-groups";

/** The Coordinator view's task filters: search, sidebar clauses, scope and status group. */
export type CoordinatorFilterState = {
  version: 1;
  query: string;
  clauses: FilterClause[];
  scope: "all" | "coordinated";
  group: CoordinatorTaskGroup | "all";
};

export const DEFAULT_COORDINATOR_FILTERS: CoordinatorFilterState = {
  version: 1,
  query: "",
  clauses: [],
  scope: "all",
  group: "all",
};

export type CatalogLookup = {
  repositorySlugById: Map<string, string>;
  repositoryIdBySlug: Map<string, string>;
};

/** Slug lookups for the workspace's repositories; a slug shared by two repositories is not reversible. */
export function catalogLookup(repositories: Repository[]): CatalogLookup {
  const repositorySlugById = new Map<string, string>();
  const repositoryIdBySlug = new Map<string, string>();
  const ambiguous = new Set<string>();
  for (const repository of repositories) {
    const slug = repositorySlug(repository);
    repositorySlugById.set(repository.id, slug);
    if (repositoryIdBySlug.has(slug)) ambiguous.add(slug);
    repositoryIdBySlug.set(slug, repository.id);
  }
  for (const slug of ambiguous) repositoryIdBySlug.delete(slug);
  return { repositorySlugById, repositoryIdBySlug };
}

/**
 * Projects a coordinator task onto the sidebar's filter item so sidebar
 * clauses apply unchanged. Pull request info and the repository path are
 * derived exactly as the sidebar derives them, so a copied clause matches the
 * same tasks in both places.
 */
export function toFilterItem(task: Task, lookup: CatalogLookup): TaskSwitcherItem {
  const summary = task.status_summary;
  const repositories = [...(task.repositories ?? [])]
    .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
    .map((repo) => lookup.repositorySlugById.get(repo.repository_id))
    .filter((slug): slug is string => !!slug);
  const git = summary?.git;
  return {
    id: task.id,
    title: task.title,
    state: task.state,
    sessionState: summary
      ? summary.primary_session?.state
      : (task.primary_session_state ?? undefined),
    workflowId: task.workflow_id,
    workflowStepId: task.workflow_step_id,
    remoteExecutorType: task.primary_executor_type ?? undefined,
    repositoryPath: repositoryPathFromSummary(summary) ?? repositories[0],
    repositories,
    diffStats: git ? { additions: git.additions ?? 0, deletions: git.deletions ?? 0 } : undefined,
    prInfo: taskPRInfoFromSummary(summary),
    isPRReview: isPRReviewFromMetadata(task.metadata),
    isIssueWatch: isIssueWatchFromMetadata(task.metadata),
    isArchived: task.archived_at != null,
    createdAt: task.created_at,
    updatedAt: task.updated_at,
    lastActivityAt: summary?.last_activity_at,
  };
}

/** A clause still being filled in (no value yet) does not filter anything. */
export function isCompleteClause(clause: FilterClause) {
  if (typeof clause.value === "boolean") return true;
  if (Array.isArray(clause.value)) return clause.value.length > 0;
  return clause.value.trim() !== "";
}

export function activeClauses(clauses: FilterClause[]) {
  return clauses.filter(isCompleteClause);
}

function singlePositive(clauses: FilterClause[], dimension: FilterClause["dimension"]) {
  const matching = clauses.filter((clause) => clause.dimension === dimension);
  const [only] = matching;
  if (matching.length !== 1 || only.op !== "is" || typeof only.value !== "string") return "";
  return only.value;
}

const requiresArchived = (clause: FilterClause) =>
  (clause.op === "is" && clause.value === true) ||
  (clause.op === "is_not" && clause.value === false);

function archivedParams(clauses: FilterClause[]) {
  if (!viewRequiresArchivedTasks({ filters: clauses })) return {};
  const archived = clauses.filter((clause) => clause.dimension === "archived");
  return archived.every(requiresArchived) ? { onlyArchived: true } : { includeArchived: true };
}

/**
 * The server-side part of the filters. Only an unambiguous single positive
 * workflow or repository clause is pushed down; every clause still applies on
 * the client, so a pushed-down clause only narrows the read.
 */
export function serverParams(state: CoordinatorFilterState, lookup: CatalogLookup) {
  const clauses = activeClauses(state.clauses);
  const params: {
    query?: string;
    workflowId?: string;
    repositoryId?: string;
    includeArchived?: boolean;
    onlyArchived?: boolean;
  } = { ...archivedParams(clauses) };
  if (state.query.trim()) params.query = state.query;
  const workflowId = singlePositive(clauses, "workflow");
  if (workflowId) params.workflowId = workflowId;
  const repositoryId = lookup.repositoryIdBySlug.get(singlePositive(clauses, "repository"));
  if (repositoryId) params.repositoryId = repositoryId;
  return params;
}

/** Applies the scope and the sidebar clauses, keeping the rows' order. */
export function applyCoordinatorFilters(
  tasks: Task[],
  state: CoordinatorFilterState,
  selectedOrchestratorId: string,
  lookup: CatalogLookup,
): Task[] {
  const scoped =
    state.scope === "coordinated" && selectedOrchestratorId
      ? tasks.filter((task) => task.metadata?.orchestration_chief_id === selectedOrchestratorId)
      : tasks;
  const clauses = activeClauses(state.clauses);
  if (clauses.length === 0) return scoped;
  const kept = new Set(
    applyFilters(
      scoped.map((task) => toFilterItem(task, lookup)),
      clauses,
    ).map((item) => item.id),
  );
  return scoped.filter((task) => kept.has(task.id));
}

const storageKey = (workspaceId: string) => `kandev.coordinator.filters.v1.${workspaceId}`;

function validClause(value: unknown): value is FilterClause {
  if (!value || typeof value !== "object") return false;
  const clause = value as Partial<FilterClause>;
  const meta = DIMENSION_METAS.find((item) => item.dimension === clause.dimension);
  const typed = ["string", "boolean"].includes(typeof clause.value) || Array.isArray(clause.value);
  return !!meta && typeof clause.id === "string" && meta.ops.includes(clause.op!) && typed;
}

/** Restores a workspace's saved filters; anything unknown falls back to the defaults. */
export function loadCoordinatorFilters(workspaceId: string): CoordinatorFilterState {
  const saved = getLocalStorage<Record<string, never> | null>(
    storageKey(workspaceId),
    null,
  ) as Partial<CoordinatorFilterState> | null;
  if (!saved || saved.version !== 1) return DEFAULT_COORDINATOR_FILTERS;
  const group = COORDINATOR_GROUPS.find((item) => item === saved.group) ?? "all";
  return {
    version: 1,
    query: typeof saved.query === "string" ? saved.query : "",
    clauses: Array.isArray(saved.clauses) ? saved.clauses.filter(validClause) : [],
    scope: saved.scope === "coordinated" ? "coordinated" : "all",
    group,
  };
}

export function saveCoordinatorFilters(workspaceId: string, state: CoordinatorFilterState) {
  setLocalStorage(storageKey(workspaceId), state);
}
