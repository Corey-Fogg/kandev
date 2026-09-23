import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  features: { orchestration: true },
  workspaces: { activeId: "ws 1" as string | null },
  kanban: { tasks: [] as unknown[] },
  kanbanMulti: { snapshots: {} },
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (s: typeof state) => unknown) => select(state),
}));
vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/",
  useRouter: () => ({ push: vi.fn() }),
}));

import { OrchestrationNav } from "./orchestration-nav";

const coordinated = (id: string, pending?: string) => ({
  id,
  metadata: { orchestration_chief_id: "chief" },
  statusSummary: pending ? { pending_action: pending } : undefined,
});
const renderNav = () =>
  render(
    <TooltipProvider>
      <OrchestrationNav />
    </TooltipProvider>,
  );

beforeEach(() => {
  state.features.orchestration = true;
  state.workspaces.activeId = "ws 1";
  state.kanban.tasks = [];
});
afterEach(cleanup);

describe("OrchestrationNav", () => {
  it("links the active workspace's coordinator", () => {
    renderNav();
    expect(screen.getByTestId("workspace-coordinator-link").getAttribute("href")).toBe(
      "/workspaces/ws%201/coordinator",
    );
  });

  it("renders nothing while orchestration is off or no workspace is active", () => {
    state.features.orchestration = false;
    const { container, rerender } = renderNav();
    expect(container.textContent).toBe("");
    state.features.orchestration = true;
    state.workspaces.activeId = null;
    rerender(
      <TooltipProvider>
        <OrchestrationNav />
      </TooltipProvider>,
    );
    expect(screen.queryByTestId("workspace-coordinator-link")).toBeNull();
  });

  it("badges delegated tasks that wait on an answer", () => {
    state.kanban.tasks = [
      coordinated("a", "clarification"),
      coordinated("b", "permission"),
      coordinated("c"),
    ];
    renderNav();
    expect(screen.getByTestId("workspace-coordinator-link").textContent).toContain("2");
  });
});
