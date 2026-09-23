import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SEARCH_DEBOUNCE_MS, useCoordinatorTasks } from "./use-coordinator-tasks";

const load = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/kanban-api", () => ({ listTasksByWorkspace: load }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({ connection: { status: "disconnected" }, auth: { user: { id: "u" } } }),
}));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => null }));
vi.mock("@/hooks/use-foreground-refresh", () => ({ useForegroundRefresh: () => {} }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

it("reads once per settled search instead of once per keystroke", async () => {
  vi.useFakeTimers();
  load.mockResolvedValue({ tasks: [], total: 0 });
  const { rerender } = renderHook(({ query }) => useCoordinatorTasks("ws", { query }), {
    initialProps: { query: "" },
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  const initial = load.mock.calls.length;
  for (const query of ["f", "fi", "fix"]) {
    rerender({ query });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS / 3);
    });
  }
  expect(load).toHaveBeenCalledTimes(initial);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
  });
  expect(load).toHaveBeenCalledTimes(initial + 1);
  expect(load.mock.lastCall?.[1]).toMatchObject({ query: "fix" });
});
