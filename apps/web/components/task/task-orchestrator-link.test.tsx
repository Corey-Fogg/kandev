import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/lib/types/http";
import { MobileTaskOrchestratorLink, OrchestratedTaskFrame } from "./task-orchestrator-link";

const feature = vi.hoisted(() => ({ orchestration: true }));
vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => feature.orchestration,
}));

const task = (chief?: string) =>
  ({
    id: "t",
    workspace_id: "ws",
    metadata: chief ? { orchestration_chief_id: chief } : {},
  }) as unknown as Task;
const body = <p data-testid="task-body">Body</p>;

afterEach(() => {
  cleanup();
  feature.orchestration = true;
});

it("renders a plain task body unchanged", () => {
  const { container } = render(<OrchestratedTaskFrame task={task()}>{body}</OrchestratedTaskFrame>);
  expect(container.firstElementChild?.getAttribute("data-testid")).toBe("task-body");
  expect(screen.queryByRole("link")).toBeNull();
});

it("renders the body unchanged while orchestration is off", () => {
  feature.orchestration = false;
  const { container } = render(
    <OrchestratedTaskFrame task={task("chief")}>{body}</OrchestratedTaskFrame>,
  );
  expect(container.firstElementChild?.getAttribute("data-testid")).toBe("task-body");
});

it("links a coordinated task back to its coordinator, including on mobile", () => {
  render(
    <OrchestratedTaskFrame task={task("chief")}>
      {body}
      <MobileTaskOrchestratorLink />
    </OrchestratedTaskFrame>,
  );
  const links = screen.getAllByRole("link");
  expect(links).toHaveLength(2);
  for (const link of links)
    expect(link.getAttribute("href")).toBe("/settings/workspaces/ws/orchestration/chief");
  expect(screen.getByTestId("task-body")).toBeTruthy();
});

it("renders no mobile link outside a coordinated frame", () => {
  render(<MobileTaskOrchestratorLink />);
  expect(screen.queryByRole("link")).toBeNull();
});
