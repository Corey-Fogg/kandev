import { afterEach, describe, expect, it, vi } from "vitest";
import {
  invalidateWorkspaceOrchestrators,
  readWorkspaceOrchestrators,
} from "./orchestrator-list-cache";

const list = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/orchestration-api", () => ({ listOrchestrators: list }));

afterEach(() => {
  invalidateWorkspaceOrchestrators();
  vi.clearAllMocks();
});

describe("workspace orchestrator list cache", () => {
  it("shares one read per workspace while it is fresh", async () => {
    list.mockResolvedValue({ orchestrators: [] });
    const first = readWorkspaceOrchestrators("ws", 0);
    expect(readWorkspaceOrchestrators("ws", 5_000)).toBe(first);
    readWorkspaceOrchestrators("other", 0);
    expect(list).toHaveBeenCalledTimes(2);
    readWorkspaceOrchestrators("ws", 20_000);
    expect(list).toHaveBeenCalledTimes(3);
    await first;
  });

  it("never reuses a failed read", async () => {
    list.mockRejectedValueOnce(new Error("offline")).mockResolvedValue({ orchestrators: [] });
    await expect(readWorkspaceOrchestrators("ws", 0)).rejects.toThrow("offline");
    await readWorkspaceOrchestrators("ws", 1);
    expect(list).toHaveBeenCalledTimes(2);
  });

  it("re-reads after invalidation", async () => {
    list.mockResolvedValue({ orchestrators: [] });
    await readWorkspaceOrchestrators("ws", 0);
    invalidateWorkspaceOrchestrators("ws");
    await readWorkspaceOrchestrators("ws", 1);
    expect(list).toHaveBeenCalledTimes(2);
  });
});
