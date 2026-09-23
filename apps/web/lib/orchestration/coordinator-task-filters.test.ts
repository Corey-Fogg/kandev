import { afterEach, describe, expect, it } from "vitest";
import type { FilterClause } from "@/lib/state/slices/ui/sidebar-view-types";
import type { Repository, Task } from "@/lib/types/http";
import {
  DEFAULT_COORDINATOR_FILTERS,
  applyCoordinatorFilters,
  catalogLookup,
  loadCoordinatorFilters,
  saveCoordinatorFilters,
  serverParams,
  toFilterItem,
  type CoordinatorFilterState,
} from "./coordinator-task-filters";

const UPDATED = "2026-09-02T00:00:00Z";
const repo = (id: string, owner: string, name: string) =>
  ({
    id,
    name,
    provider_owner: owner,
    provider_name: name,
    local_path: `/src/${name}`,
  }) as Repository;
const lookup = catalogLookup([repo("r1", "acme", "api"), repo("r2", "acme", "web")]);
const task = (id: string, patch: Partial<Task> = {}) =>
  ({
    id,
    title: `Task ${id}`,
    state: "TODO",
    workflow_id: "wf",
    workflow_step_id: "todo",
    created_at: "2026-09-01T00:00:00Z",
    updated_at: UPDATED,
    metadata: {},
    ...patch,
  }) as Task;
const clause = (patch: Partial<FilterClause>): FilterClause =>
  ({ id: "c", dimension: "workflow", op: "is", value: "", ...patch }) as FilterClause;
const state = (patch: Partial<CoordinatorFilterState>): CoordinatorFilterState => ({
  ...DEFAULT_COORDINATOR_FILTERS,
  ...patch,
});
const filterIds = (tasks: Task[], patch: Partial<CoordinatorFilterState>, selected = "") =>
  applyCoordinatorFilters(tasks, state(patch), selected, lookup).map((item) => item.id);

describe("toFilterItem", () => {
  it("maps session state, repository slugs, diff, pull request and archive state", () => {
    const item = toFilterItem(
      task("a", {
        primary_session_state: "COMPLETED",
        repositories: [
          { repository_id: "r2", position: 1 },
          { repository_id: "r1", position: 0 },
        ] as Task["repositories"],
        archived_at: "2026-09-03T00:00:00Z",
        primary_executor_type: "local",
        status_summary: {
          revision: 1,
          updated_at: UPDATED,
          last_activity_at: "2026-09-02T01:00:00Z",
          primary_session: { id: "s", state: "RUNNING" },
          git: { additions: 3, deletions: 1 },
          pull_request: { number: 9, state: "open" },
        },
      }),
      lookup,
    );
    expect(item).toMatchObject({
      sessionState: "RUNNING",
      repositoryPath: "acme/api",
      repositories: ["acme/api", "acme/web"],
      diffStats: { additions: 3, deletions: 1 },
      prInfo: { number: 9, state: "Open" },
      isArchived: true,
      remoteExecutorType: "local",
      lastActivityAt: "2026-09-02T01:00:00Z",
    });
    const bare = toFilterItem(task("b", { primary_session_state: "WAITING_FOR_INPUT" }), lookup);
    expect(bare).toMatchObject({ sessionState: "WAITING_FOR_INPUT", isArchived: false });
    expect(bare.prInfo).toBeUndefined();
    expect(bare.diffStats).toBeUndefined();
  });

  it("derives pull request info and the repository path as the sidebar does", () => {
    const summary = { revision: 1, updated_at: UPDATED };
    const aggregate = toFilterItem(
      task("agg", {
        repositories: [{ repository_id: "r1", position: 0 }] as Task["repositories"],
        status_summary: { ...summary, pull_request: { count: 2, aggregate_state: "open" } },
      }),
      lookup,
    );
    expect(aggregate.prInfo, "a summary without a number has no pull request").toBeUndefined();
    expect(aggregate.repositoryPath).toBe("acme/api");
    const fork = toFilterItem(
      task("fork", {
        repositories: [{ repository_id: "r1", position: 0 }] as Task["repositories"],
        status_summary: {
          ...summary,
          pull_request: { number: 4, state: "open", url: "https://github.com/fork/api/pull/4" },
        },
      }),
      lookup,
    );
    expect(fork.repositoryPath, "the pull request URL names the repository first").toBe("fork/api");
  });
});

describe("serverParams", () => {
  it("pushes down a single positive workflow or repository clause", () => {
    expect(serverParams(state({ clauses: [clause({ value: "wf" })] }), lookup)).toEqual({
      workflowId: "wf",
    });
    expect(
      serverParams(
        state({ clauses: [clause({ dimension: "repository", value: "acme/web" })] }),
        lookup,
      ),
    ).toEqual({ repositoryId: "r2" });
  });

  it("keeps multi-value, negative and repeated clauses on the client", () => {
    const params = (clauses: FilterClause[]) => serverParams(state({ clauses }), lookup);
    expect(params([clause({ op: "in", value: ["wf"] })])).toEqual({});
    expect(params([clause({ op: "is_not", value: "wf" })])).toEqual({});
    expect(params([clause({ value: "wf" }), clause({ id: "d", value: "other" })])).toEqual({});
    expect(params([clause({ dimension: "repository", value: "unknown/repo" })])).toEqual({});
    expect(params([clause({ value: "" })])).toEqual({});
  });

  it("reads archived tasks only when a clause requires them", () => {
    const archived = clause({ dimension: "archived", value: true });
    expect(serverParams(state({ clauses: [archived] }), lookup)).toEqual({ onlyArchived: true });
    expect(
      serverParams(
        state({
          clauses: [archived, clause({ id: "d", dimension: "archived", op: "is", value: false })],
        }),
        lookup,
      ),
    ).toEqual({ includeArchived: true });
    expect(
      serverParams(state({ clauses: [clause({ dimension: "archived", op: "is_not" })] }), lookup),
    ).toEqual({});
    expect(serverParams(state({ query: "parser" }), lookup)).toEqual({ query: "parser" });
  });
});

describe("applyCoordinatorFilters", () => {
  const tasks = [
    task("review", { state: "REVIEW", workflow_step_id: "done" }),
    task("running", {
      state: "IN_PROGRESS",
      primary_session_state: "RUNNING",
      metadata: { orchestration_chief_id: "jeb" },
      status_summary: {
        revision: 1,
        updated_at: UPDATED,
        pull_request: { number: 3, state: "open" },
      },
    }),
    task("backlog", { title: "Write the parser docs" }),
  ];

  it("combines state bucket, step, pull request and title clauses", () => {
    expect(
      filterIds(tasks, {
        clauses: [clause({ dimension: "state", op: "in", value: ["review", "in_progress"] })],
      }),
    ).toEqual(["review", "running"]);
    expect(
      filterIds(tasks, { clauses: [clause({ dimension: "workflowStep", value: "done" })] }),
    ).toEqual(["review"]);
    expect(filterIds(tasks, { clauses: [clause({ dimension: "hasPR", value: true })] })).toEqual([
      "running",
    ]);
    expect(
      filterIds(tasks, {
        clauses: [clause({ dimension: "titleMatch", op: "matches", value: "PARSER" })],
      }),
    ).toEqual(["backlog"]);
  });

  it("scopes to the selected orchestrator and ignores incomplete clauses", () => {
    expect(filterIds(tasks, { scope: "coordinated" }, "jeb")).toEqual(["running"]);
    expect(filterIds(tasks, { scope: "coordinated" })).toHaveLength(3);
    expect(filterIds(tasks, { clauses: [clause({ value: "" })] })).toHaveLength(3);
  });
});

describe("persistence", () => {
  afterEach(() => window.localStorage.clear());

  it("round-trips per workspace", () => {
    const saved = state({
      query: "parser",
      scope: "coordinated",
      group: "review",
      clauses: [clause({ value: "wf" })],
    });
    saveCoordinatorFilters("ws-1", saved);
    expect(loadCoordinatorFilters("ws-1")).toEqual(saved);
    expect(loadCoordinatorFilters("ws-2")).toEqual(DEFAULT_COORDINATOR_FILTERS);
  });

  it("drops invalid clauses and ignores another version", () => {
    window.localStorage.setItem(
      "kandev.coordinator.filters.v1.ws",
      JSON.stringify({
        version: 1,
        query: 3,
        scope: "weird",
        group: "nope",
        clauses: [
          clause({ value: "wf" }),
          { id: "x", dimension: "priority", op: "is", value: "high" },
          { id: "y", dimension: "titleMatch", op: "is", value: "x" },
        ],
      }),
    );
    expect(loadCoordinatorFilters("ws")).toEqual(state({ clauses: [clause({ value: "wf" })] }));
    window.localStorage.setItem("kandev.coordinator.filters.v1.ws", JSON.stringify({ version: 2 }));
    expect(loadCoordinatorFilters("ws")).toEqual(DEFAULT_COORDINATOR_FILTERS);
  });
});
