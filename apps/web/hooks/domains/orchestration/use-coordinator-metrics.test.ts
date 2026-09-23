import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { CoordinatorMetrics } from "@/lib/api/domains/orchestration-api";

const api = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock("@/lib/api/domains/orchestration-api", () => ({ getOrchestratorMetrics: api.get }));

import { useCoordinatorMetrics } from "./use-coordinator-metrics";

const metrics = (days: number, completed: number) =>
  ({ days, completed }) as unknown as CoordinatorMetrics;
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

afterEach(() => api.get.mockReset());

it("reads the requested window and re-reads when the window changes", async () => {
  api.get.mockImplementation(async (_ws: string, _id: string, days: number) => metrics(days, days));
  const { result, rerender } = renderHook(
    ({ days }: { days: 7 | 30 }) => useCoordinatorMetrics("ws", "jeb", days),
    { initialProps: { days: 7 as 7 | 30 } },
  );
  expect(result.current.loading).toBe(true);
  await waitFor(() => expect(result.current.data?.days).toBe(7));
  rerender({ days: 30 });
  expect(result.current.data).toBeUndefined();
  await waitFor(() => expect(result.current.data?.days).toBe(30));
  expect(api.get).toHaveBeenLastCalledWith("ws", "jeb", 30, expect.anything());
});

it("rejects a stale response after the orchestrator changes", async () => {
  const slow = deferred<CoordinatorMetrics>();
  api.get.mockImplementation((_ws: string, id: string) =>
    id === "old" ? slow.promise : Promise.resolve(metrics(7, 3)),
  );
  const { result, rerender } = renderHook(
    ({ id }: { id: string }) => useCoordinatorMetrics("ws", id, 7),
    { initialProps: { id: "old" } },
  );
  rerender({ id: "new" });
  await waitFor(() => expect(result.current.data?.completed).toBe(3));
  await act(async () => slow.resolve(metrics(7, 99)));
  expect(result.current.data?.completed).toBe(3);
});

it("reports an error and recovers on refresh", async () => {
  api.get.mockRejectedValueOnce(new Error("down")).mockResolvedValueOnce(metrics(7, 1));
  const { result } = renderHook(() => useCoordinatorMetrics("ws", "jeb", 7));
  await waitFor(() => expect(result.current.error).toBeTruthy());
  act(() => result.current.refresh());
  await waitFor(() => expect(result.current.data?.completed).toBe(1));
});

it("reads nothing without an orchestrator", () => {
  const { result } = renderHook(() => useCoordinatorMetrics("ws", "", 7));
  expect(result.current.loading).toBe(false);
  expect(api.get).not.toHaveBeenCalled();
});
