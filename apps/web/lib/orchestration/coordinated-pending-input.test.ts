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
});
