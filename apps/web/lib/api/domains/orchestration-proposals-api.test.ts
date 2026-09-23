import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  approveTaskProposal,
  dismissTaskProposal,
  getTaskProposal,
  listTaskProposals,
} from "./orchestration-proposals-api";
import { getOrchestratorMetrics, patchOrchestrator } from "./orchestration-api";

const fetcher = vi.fn();
beforeEach(() => {
  fetcher.mockImplementation(async () => new Response("{}", { status: 200 }));
  vi.stubGlobal("fetch", fetcher);
});
afterEach(() => {
  vi.unstubAllGlobals();
  fetcher.mockReset();
});

const call = (index = 0) => {
  const [url, init] = fetcher.mock.calls[index] as [string, RequestInit | undefined];
  return {
    path: new URL(url, "http://host").pathname,
    search: new URL(url, "http://host").search,
    method: init?.method ?? "GET",
    body: init?.body ? JSON.parse(String(init.body)) : undefined,
  };
};
const base = "/api/v1/orchestration/workspaces/w%201/orchestrators/o%2F1/proposals";

it("lists and reads proposals with encoded path segments", async () => {
  await listTaskProposals("w 1", "o/1", "pending");
  expect(call()).toMatchObject({ path: base, method: "GET" });
  expect(call().search).toBe("?status=pending&limit=50");
  await listTaskProposals("w 1", "o/1");
  expect(call(1).search).toBe("?status=all&limit=50");
  await getTaskProposal("w 1", "o/1", "p#1");
  expect(call(2)).toMatchObject({ path: `${base}/p%231`, method: "GET" });
});

it("approves with only the changed fields and sends an empty body otherwise", async () => {
  await approveTaskProposal("w 1", "o/1", "p1");
  expect(call()).toMatchObject({ path: `${base}/p1/approve`, method: "POST", body: {} });
  await approveTaskProposal("w 1", "o/1", "p1", { title: "Renamed", acceptance_criteria: ["a"] });
  expect(call(1).body).toEqual({ edits: { title: "Renamed", acceptance_criteria: ["a"] } });
  await approveTaskProposal("w 1", "o/1", "p1", {});
  expect(call(2).body).toEqual({});
});

it("dismisses with a trimmed optional reason", async () => {
  await dismissTaskProposal("w 1", "o/1", "p1", "  not now ");
  expect(call()).toMatchObject({
    path: `${base}/p1/dismiss`,
    method: "POST",
    body: { reason: "not now" },
  });
  await dismissTaskProposal("w 1", "o/1", "p1", "   ");
  expect(call(1).body).toEqual({});
});

it("patches orchestrator identity and reads metrics by window", async () => {
  await patchOrchestrator("w 1", "o/1", { display_name: "Jeb" });
  expect(call()).toMatchObject({
    path: "/api/v1/orchestration/workspaces/w%201/orchestrators/o%2F1",
    method: "PATCH",
    body: { display_name: "Jeb" },
  });
  await getOrchestratorMetrics("w 1", "o/1", 30);
  expect(call(1)).toMatchObject({
    path: "/api/v1/orchestration/workspaces/w%201/orchestrators/o%2F1/metrics",
    method: "GET",
  });
  expect(call(1).search).toBe("?days=30");
});
