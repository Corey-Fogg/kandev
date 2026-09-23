import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { AutomationRun } from "@/lib/types/automation";
import { RunFeedItem } from "./run-feed-item";

const run = (patch: Partial<AutomationRun>) =>
  ({
    id: "r",
    automation_id: "a",
    trigger_id: "tr",
    trigger_type: "scheduled",
    task_id: "",
    status: "succeeded",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: "2026-09-23T00:00:00Z",
    ...patch,
  }) as AutomationRun;

afterEach(cleanup);

it("opens an orchestrator delivery in its conversation", () => {
  const onOpen = vi.fn();
  render(
    <RunFeedItem
      run={run({ status: "dispatched", conversation_task_id: "c 1" })}
      onOpen={onOpen}
    />,
  );
  expect(screen.getByTestId("run-entry-r").getAttribute("href")).toBe(
    "/workspace/conversations/c%201",
  );
});

it("opens a task run's transcript", () => {
  const onOpen = vi.fn();
  render(<RunFeedItem run={run({ task_id: "t" })} onOpen={onOpen} />);
  fireEvent.click(screen.getByTestId("run-entry-r"));
  expect(onOpen).toHaveBeenCalledWith("t");
});
