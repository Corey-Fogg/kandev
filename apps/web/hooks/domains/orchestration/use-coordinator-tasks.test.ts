import { describe, expect, it, vi } from "vitest";
import type { Task, ListTasksResponse } from "@/lib/types/http";
import { ApiError } from "@/lib/api/client";
import { CoordinatorTaskObservation } from "@/lib/orchestration/coordinator-task-observation";

const row = (id: string, revision = 1): Task =>
  ({
    id,
    workspace_id: "ws",
    title: id,
    state: "CREATED",
    status_summary: { revision, updated_at: "2026-09-17T00:00:00Z" },
  }) as Task;
const deferred = () => {
  let resolve!: (v: ListTasksResponse) => void;
  const promise = new Promise<ListTasksResponse>((r) => {
    resolve = r;
  });
  return { promise, resolve };
};
const page = (tasks: Task[], total = tasks.length) => ({ tasks, total });

describe("coordinator task observations", () => {
  it("loads additional pages without claiming complete counts", async () => {
    const first = Array.from({ length: 100 }, (_, i) => row(String(i)));
    const load = vi
      .fn()
      .mockResolvedValueOnce(page(first, 101))
      .mockResolvedValueOnce(page(first, 101))
      .mockResolvedValueOnce(page([row("100")], 101));
    const view = new CoordinatorTaskObservation("ws", {}, load);
    await view.refresh();
    expect(view.getSnapshot().tasks).toHaveLength(100);
    expect(view.getSnapshot().complete).toBe(false);
    await view.loadMore();
    expect(view.getSnapshot().tasks).toHaveLength(101);
    expect(view.getSnapshot().complete).toBe(true);
    expect(load.mock.calls[2][1]).toMatchObject({
      page: 2,
      pageSize: 100,
      excludeConfig: true,
      view: "kanban",
    });
    view.dispose();
  });
  it("rejects older HTTP status after a WS delta, including during the first load", async () => {
    const pending = deferred();
    const view = new CoordinatorTaskObservation("ws", {}, vi.fn().mockReturnValue(pending.promise));
    const request = view.refresh();
    view.applySummary({
      task_id: "t",
      workspace_id: "ws",
      status_summary: { revision: 9, updated_at: "now", pending_action: "permission" },
    });
    pending.resolve(page([row("t", 2)]));
    await request;
    expect(view.getSnapshot().tasks[0].status_summary?.revision).toBe(9);
    view.dispose();
  });
  it("preserves an equal-revision queue count refresh", async () => {
    const load = vi
      .fn()
      .mockResolvedValueOnce(page([row("t")]))
      .mockResolvedValueOnce(
        page([
          { ...row("t"), status_summary: { ...row("t").status_summary!, queued_prompt_count: 2 } },
        ]),
      );
    const view = new CoordinatorTaskObservation("ws", {}, load);
    await view.refresh();
    await view.refresh();
    expect(view.getSnapshot().tasks[0].status_summary?.queued_prompt_count).toBe(2);
    view.dispose();
  });
});

describe("coordinator scope recovery", () => {
  it("clears old workspace before a delayed response and aborts the old request", async () => {
    const pending = deferred();
    const load = vi.fn().mockReturnValue(pending.promise);
    const old = new CoordinatorTaskObservation("ws", {}, load);
    const request = old.refresh();
    old.dispose();
    const next = new CoordinatorTaskObservation("other", {}, vi.fn().mockResolvedValue(page([])));
    expect(next.getSnapshot().tasks).toEqual([]);
    expect(load.mock.calls[0][2].init.signal.aborted).toBe(true);
    pending.resolve(page([row("private-old")]));
    await request;
    expect(old.getSnapshot().tasks).toEqual([]);
    expect(next.getSnapshot().tasks).toEqual([]);
    next.dispose();
  });
  it("read failure retains stale coverage; denied access clears it", async () => {
    const load = vi
      .fn()
      .mockResolvedValueOnce(page([row("t")]))
      .mockRejectedValueOnce(new Error("Unavailable"))
      .mockRejectedValueOnce(new ApiError("Not found", 404, {}));
    const view = new CoordinatorTaskObservation("ws", {}, load);
    await view.refresh();
    await view.refresh();
    expect(view.getSnapshot().tasks).toHaveLength(1);
    expect(view.getSnapshot().stale).toBe(true);
    await view.refresh();
    expect(view.getSnapshot().tasks).toHaveLength(0);
    view.dispose();
  });
  it("reconciles changed and deleted tasks on reconnect without accepting foreign deltas", async () => {
    const load = vi
      .fn()
      .mockResolvedValueOnce(page([row("t"), row("deleted")]))
      .mockResolvedValueOnce(page([row("t", 5)]));
    const view = new CoordinatorTaskObservation(
      "ws",
      { query: "example", workflowId: "wf", repositoryId: "repo" },
      load,
    );
    await view.refresh();
    view.applySummary({
      task_id: "t",
      workspace_id: "foreign",
      status_summary: { revision: 99, updated_at: "now" },
    });
    expect(view.getSnapshot().tasks[0].status_summary?.revision).toBe(1);
    await view.refresh();
    expect(view.getSnapshot().tasks.map((task) => task.id)).toEqual(["t"]);
    expect(load.mock.calls[0][1]).toMatchObject({
      query: "example",
      workflowId: "wf",
      repositoryId: "repo",
    });
    view.dispose();
  });
  it("removes deletions immediately and never resurrects them from an older read", async () => {
    const pending = deferred();
    const load = vi
      .fn()
      .mockResolvedValueOnce(page([row("t")]))
      .mockReturnValueOnce(pending.promise);
    const view = new CoordinatorTaskObservation("ws", {}, load);
    await view.refresh();
    const request = view.refresh();
    view.lifecycle({ task_id: "t", workspace_id: "ws" }, true);
    expect(view.getSnapshot().tasks).toEqual([]);
    pending.resolve(page([row("t")]));
    await request;
    expect(view.getSnapshot().tasks).toEqual([]);
    view.dispose();
  });
});

describe("coordinator task list updates", () => {
  it("keeps the current rows visible while a new filter is read", async () => {
    const pending = deferred();
    const load = vi
      .fn()
      .mockResolvedValueOnce(page([row("a"), row("b")]))
      .mockReturnValueOnce(pending.promise);
    const view = new CoordinatorTaskObservation("ws", {}, load);
    view.setFilters({ query: "" });
    await vi.waitFor(() => expect(view.getSnapshot().tasks).toHaveLength(2));
    view.setFilters({ query: "" });
    expect(load).toHaveBeenCalledTimes(1);
    view.setFilters({ query: "a" });
    expect(view.getSnapshot().tasks).toHaveLength(2);
    expect(load.mock.calls[1][1]).toMatchObject({ query: "a", page: 1 });
    pending.resolve(page([row("a")]));
    await vi.waitFor(() => expect(view.getSnapshot().tasks).toHaveLength(1));
    view.dispose();
  });

  it("commits a read that raced a lifecycle event, then reads once more", async () => {
    vi.useFakeTimers();
    const pending = deferred();
    const load = vi
      .fn()
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValueOnce(page([row("a", 2), row("b")]));
    const view = new CoordinatorTaskObservation("ws", {}, load);
    const request = view.refresh();
    view.lifecycle({ task_id: "b", workspace_id: "ws" });
    pending.resolve(page([row("a")]));
    await request;
    expect(view.getSnapshot().tasks.map((task) => task.id)).toEqual(["a"]);
    await vi.advanceTimersByTimeAsync(250);
    expect(view.getSnapshot().tasks.map((task) => task.id)).toEqual(["a", "b"]);
    view.dispose();
    vi.useRealTimers();
  });

  it("keeps complete coverage when a loaded task is deleted", async () => {
    const view = new CoordinatorTaskObservation(
      "ws",
      {},
      vi.fn().mockResolvedValue(page([row("a"), row("b")])),
    );
    await view.refresh();
    view.lifecycle({ task_id: "a", workspace_id: "ws" }, true);
    expect(view.getSnapshot()).toMatchObject({ complete: true, total: 1 });
    view.dispose();
  });
});
