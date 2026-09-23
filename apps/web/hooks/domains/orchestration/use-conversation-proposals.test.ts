import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { TaskComment } from "@/app/office/tasks/[id]/types";
import type { TaskProposal } from "@/lib/api/domains/orchestration-proposals-api";

const api = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn() }));
vi.mock("@/lib/api/domains/orchestration-proposals-api", () => ({
  listTaskProposals: api.list,
  getTaskProposal: api.get,
}));

import { mergeProposal, useConversationProposals } from "./use-conversation-proposals";

const comment = (id: string, createdAt: string, source?: string) =>
  ({ id, createdAt, source, authorType: "agent", content: "" }) as TaskComment;
const proposal = (id: string, status: TaskProposal["status"] = "pending") =>
  ({ id, status, spec: { title: id } }) as TaskProposal;
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
type Props = { ws: string; id: string; comments: TaskComment[] };
const render = (initialProps: Props) =>
  renderHook(({ ws, id, comments }: Props) => useConversationProposals(ws, id, comments), {
    initialProps,
  });

afterEach(() => {
  api.list.mockReset();
  api.get.mockReset();
});

it("re-reads when a new proposal comment appears and counts pending cards", async () => {
  api.list.mockResolvedValueOnce({ proposals: [proposal("p1")] });
  const first = [comment("p1", "2026-09-23T10:00:00Z", "proposal")];
  const { result, rerender } = render({ ws: "ws", id: "jeb", comments: first });
  await waitFor(() => expect(result.current.pendingCount).toBe(1));
  expect(result.current.firstPendingId).toBe("p1");
  api.list.mockResolvedValueOnce({ proposals: [proposal("p2"), proposal("p1", "approved")] });
  const second = [...first, comment("p2", "2026-09-23T11:00:00Z", "proposal")];
  rerender({ ws: "ws", id: "jeb", comments: second });
  await waitFor(() => expect(result.current.byId.get("p1")?.status).toBe("approved"));
  expect(api.list).toHaveBeenCalledTimes(2);
  expect(result.current.pendingCount).toBe(1);
  expect(result.current.firstPendingId).toBe("p2");
});

it("reads an older proposal missing from the list only once", async () => {
  api.list.mockResolvedValue({ proposals: [] });
  api.get.mockResolvedValue(proposal("old", "dismissed"));
  const comments = [comment("old", "2026-09-01T10:00:00Z", "proposal")];
  const { result, rerender } = render({ ws: "ws", id: "jeb", comments });
  await waitFor(() => expect(result.current.byId.get("old")?.status).toBe("dismissed"));
  rerender({ ws: "ws", id: "jeb", comments: [...comments] });
  await waitFor(() => expect(api.list).toHaveBeenCalled());
  expect(api.get).toHaveBeenCalledTimes(1);
  expect(api.get).toHaveBeenCalledWith("ws", "jeb", "old");
});

it("rejects a stale list after the orchestrator changes", async () => {
  const stale = deferred<{ proposals: TaskProposal[] }>();
  api.list.mockImplementation((_ws: string, id: string) =>
    id === "old" ? stale.promise : Promise.resolve({ proposals: [proposal("fresh")] }),
  );
  const comments = [comment("fresh", "2026-09-23T10:00:00Z", "proposal")];
  const { result, rerender } = render({ ws: "ws", id: "old", comments });
  rerender({ ws: "ws", id: "new", comments });
  await waitFor(() => expect(result.current.byId.has("fresh")).toBe(true));
  await act(async () => stale.resolve({ proposals: [proposal("leaked")] }));
  expect(result.current.byId.has("leaked")).toBe(false);
});

it("never moves a decided proposal back to pending", () => {
  expect(mergeProposal(proposal("p", "approved"), proposal("p", "pending")).status).toBe(
    "approved",
  );
  expect(mergeProposal(proposal("p", "pending"), proposal("p", "dismissed")).status).toBe(
    "dismissed",
  );
});
