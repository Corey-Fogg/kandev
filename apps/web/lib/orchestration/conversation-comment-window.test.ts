import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { PagedWindow, type WindowPage } from "./conversation-comment-window";

type Row = { id: string; body: string };
const row = (id: string, body = id): Row => ({ id, body });

describe("paged comment window", () => {
  it("keeps every loaded page on refresh and reuses unchanged rows", async () => {
    const pages: Record<string, WindowPage<Row>> = {
      "": { entries: [row("c3"), row("c2")], next_cursor: "c2" },
      c2: { entries: [row("c1")], next_cursor: "" },
    };
    const load = vi.fn(async (after: string) => pages[after]);
    const view = new PagedWindow<Row>(load);
    await view.refresh();
    expect(view.getSnapshot()).toMatchObject({ loaded: true, nextCursor: "c2" });
    await view.loadMore();
    const loaded = view.getSnapshot().entries;
    expect(loaded.map((entry) => entry.id)).toEqual(["c3", "c2", "c1"]);
    expect(view.getSnapshot().nextCursor).toBe("");

    const listener = vi.fn();
    view.subscribe(listener);
    await view.refresh();
    expect(load).toHaveBeenLastCalledWith("c2", expect.any(AbortSignal));
    expect(view.getSnapshot().entries).toBe(loaded);
    expect(listener).not.toHaveBeenCalled();

    pages[""] = { entries: [row("c4"), row("c3")], next_cursor: "c3" };
    pages.c3 = { entries: [row("c2"), row("c1")], next_cursor: "" };
    await view.refresh();
    expect(view.getSnapshot().entries.map((entry) => entry.id)).toEqual(["c4", "c3", "c2", "c1"]);
    expect(listener).toHaveBeenCalled();
    view.dispose();
  });

  it("commits only the latest read", async () => {
    let release!: (page: WindowPage<Row>) => void;
    const load = vi
      .fn()
      .mockReturnValueOnce(new Promise((resolve) => (release = resolve)))
      .mockResolvedValueOnce({ entries: [row("new")], next_cursor: "" });
    const view = new PagedWindow<Row>(load);
    const first = view.refresh();
    await view.refresh();
    release({ entries: [row("old")], next_cursor: "" });
    await first;
    expect(view.getSnapshot().entries.map((entry) => entry.id)).toEqual(["new"]);
    view.dispose();
  });

  it("keeps rows after a transient failure and clears them when access is denied", async () => {
    const load = vi
      .fn()
      .mockResolvedValueOnce({ entries: [row("c1")], next_cursor: "" })
      .mockRejectedValueOnce(new Error("offline"))
      .mockRejectedValueOnce(new ApiError("gone", 404, null));
    const view = new PagedWindow<Row>(load);
    await view.refresh();
    await view.refresh();
    expect(view.getSnapshot().entries).toHaveLength(1);
    expect(view.getSnapshot().error).toBeInstanceOf(Error);
    await view.refresh();
    expect(view.getSnapshot()).toMatchObject({ entries: [], loaded: false });
    view.dispose();
  });

  it("stops reading once disposed", async () => {
    const load = vi.fn().mockResolvedValue({ entries: [], next_cursor: "" });
    const view = new PagedWindow<Row>(load);
    view.dispose();
    await view.refresh();
    expect(load).not.toHaveBeenCalled();
  });
});
