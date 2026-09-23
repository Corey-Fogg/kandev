import { describe, expect, it } from "vitest";
import type { AppState } from "@/lib/state/app-state-types";
import { selectCoordinatedPendingInputCount } from "./coordinated-pending-input";

type StoreTask = AppState["kanban"]["tasks"][number];
const task = (id: string, chief: string | undefined, pending?: string) =>
  ({
    id,
    metadata: chief ? { orchestration_chief_id: chief } : undefined,
    statusSummary: pending ? { pending_action: pending } : undefined,
  }) as unknown as StoreTask;
const state = (tasks: StoreTask[], snapshots: StoreTask[][] = []) =>
  ({
    kanban: { tasks },
    kanbanMulti: {
      snapshots: Object.fromEntries(snapshots.map((rows, index) => [`w${index}`, { tasks: rows }])),
    },
  }) as unknown as AppState;

describe("coordinated pending input count", () => {
  it("counts coordinator-managed tasks waiting on an answer once each", () => {
    const waiting = task("a", "chief", "clarification");
    const rows = [waiting, task("b", "chief", "permission"), task("c", "chief")];
    expect(selectCoordinatedPendingInputCount(state(rows, [[waiting]]))).toBe(2);
  });

  it("ignores tasks no coordinator manages", () => {
    expect(selectCoordinatedPendingInputCount(state([task("a", undefined, "permission")]))).toBe(0);
  });

  it("counts only one orchestrator's tasks when an id is given", () => {
    const rows = [
      task("a", "chief", "clarification"),
      task("b", "other", "permission"),
      task("c", "chief", "permission"),
    ];
    expect(selectCoordinatedPendingInputCount(state(rows), "chief")).toBe(2);
    expect(selectCoordinatedPendingInputCount(state(rows), "other")).toBe(1);
    expect(selectCoordinatedPendingInputCount(state(rows), "missing")).toBe(0);
  });
});
