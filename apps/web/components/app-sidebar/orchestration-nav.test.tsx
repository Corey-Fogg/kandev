import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  features: { orchestration: true },
  workspaces: { activeId: "ws 1" as string | null },
  kanban: { tasks: [] as unknown[] },
  kanbanMulti: { snapshots: {} },
}));
const orchestrators = vi.hoisted(() => ({
  data: undefined as { orchestrators: unknown[] } | undefined,
}));
const router = vi.hoisted(() => ({ push: vi.fn(), pathname: "/", search: "" }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (s: typeof state) => unknown) => select(state),
}));
vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => router.pathname,
  useSearchParams: () => new URLSearchParams(router.search),
  useRouter: () => ({ push: router.push }),
}));
vi.mock("@/hooks/domains/orchestration/use-orchestrator-conversation", () => ({
  useWorkspaceOrchestrators: () => ({ data: orchestrators.data, refresh: vi.fn() }),
}));

import { OrchestrationNav } from "./orchestration-nav";

const GENERIC = "workspace-coordinator-link";
const JEB = "workspace-coordinator-link-jeb";

const coordinated = (id: string, chief: string, pending?: string) => ({
  id,
  metadata: { orchestration_chief_id: chief },
  statusSummary: pending ? { pending_action: pending } : undefined,
});
const orchestrator = (id: string, name: string, status = "idle") => ({ id, name, status });
const renderNav = (onNavigate?: () => void, collapsed = false) =>
  render(
    <TooltipProvider>
      <OrchestrationNav onNavigate={onNavigate} collapsed={collapsed} />
    </TooltipProvider>,
  );

beforeEach(() => {
  state.features.orchestration = true;
  state.workspaces.activeId = "ws 1";
  state.kanban.tasks = [];
  orchestrators.data = undefined;
  router.pathname = "/";
  router.search = "";
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("OrchestrationNav", () => {
  it("shows the generic coordinator entry while the list loads or is empty", () => {
    const { rerender } = renderNav();
    expect(screen.getByTestId(GENERIC).getAttribute("href")).toBe("/workspaces/ws%201/coordinator");
    orchestrators.data = { orchestrators: [] };
    rerender(
      <TooltipProvider>
        <OrchestrationNav />
      </TooltipProvider>,
    );
    expect(screen.getByTestId(GENERIC).textContent).toContain("Orchestrator");
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
    expect(screen.queryByTestId(GENERIC)).toBeNull();
  });

  it("lists each orchestrator by its name with its own link", () => {
    orchestrators.data = {
      orchestrators: [orchestrator("jeb", "Jeb"), orchestrator("old", "Ops")],
    };
    renderNav();
    expect(screen.queryByTestId(GENERIC)).toBeNull();
    const jeb = screen.getByTestId(JEB);
    expect(jeb.textContent).toContain("Jeb");
    expect(jeb.getAttribute("href")).toBe("/workspaces/ws%201/coordinator?orchestratorId=jeb");
    expect(screen.getByTestId("workspace-coordinator-link-old").textContent).toContain("Ops");
  });

  it("marks each orchestrator with its initial in the collapsed rail", () => {
    orchestrators.data = {
      orchestrators: [orchestrator("jeb", "jeb"), orchestrator("old", " Ops")],
    };
    renderNav(undefined, true);
    expect(screen.getByTestId(`${JEB}-marker`).textContent).toBe("J");
    expect(screen.getByTestId("workspace-coordinator-link-old-marker").textContent).toBe("O");
    expect(screen.getByRole("link", { name: "jeb" })).toBeTruthy();
    cleanup();
    renderNav();
    expect(screen.queryByTestId(`${JEB}-marker`)).toBeNull();
  });

  it("badges only the tasks each orchestrator delegated", () => {
    orchestrators.data = {
      orchestrators: [orchestrator("jeb", "Jeb"), orchestrator("old", "Ops")],
    };
    state.kanban.tasks = [
      coordinated("a", "jeb", "clarification"),
      coordinated("b", "jeb", "permission"),
      coordinated("c", "old", "permission"),
      coordinated("d", "old"),
    ];
    renderNav();
    expect(screen.getByTestId(JEB).textContent).toContain("2");
    expect(screen.getByTestId("workspace-coordinator-link-old").textContent).toContain("1");
    expect(screen.getByRole("link", { name: "Jeb, 2 waiting for input" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Ops, 1 waiting for input" })).toBeTruthy();
  });

  it("keeps the plain name when nothing is waiting", () => {
    orchestrators.data = { orchestrators: [orchestrator("jeb", "Jeb")] };
    renderNav();
    expect(screen.getByRole("link", { name: "Jeb" })).toBeTruthy();
  });

  it("badges delegated tasks on the generic entry", () => {
    state.kanban.tasks = [coordinated("a", "chief", "clarification"), coordinated("b", "chief")];
    renderNav();
    expect(screen.getByTestId(GENERIC).textContent).toContain("1");
  });

  it("labels a paused orchestrator", () => {
    orchestrators.data = { orchestrators: [orchestrator("jeb", "Jeb", "paused")] };
    renderNav();
    expect(screen.getByTestId(JEB).textContent).toBe("Jeb (paused)");
  });

  it("navigates and closes the mobile sheet from an orchestrator entry", () => {
    orchestrators.data = { orchestrators: [orchestrator("jeb", "Jeb")] };
    const close = vi.fn();
    renderNav(close);
    fireEvent.click(screen.getByTestId(JEB));
    expect(router.push).toHaveBeenCalledWith("/workspaces/ws%201/coordinator?orchestratorId=jeb");
    expect(close).toHaveBeenCalled();
  });
});
