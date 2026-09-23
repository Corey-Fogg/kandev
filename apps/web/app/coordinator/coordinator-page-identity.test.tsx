import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";

const workspace = vi.hoisted(() => ({ data: undefined as unknown }));
vi.mock("@/components/page-shell", () => ({
  PageShell: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (s: unknown) => unknown) =>
    select({ auth: { user: { id: "u" } }, features: { orchestration: true } }),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useSearchParams: () => new URLSearchParams(""),
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
}));
vi.mock("@/hooks/domains/orchestration/use-coordinator-workspace", () => ({
  useCoordinatorWorkspace: () => ({ data: workspace.data, refresh: vi.fn() }),
}));
vi.mock("./coordinator-task-list", () => ({ CoordinatorTaskList: () => null }));
vi.mock("./coordinator-chat", () => ({ CoordinatorChat: () => null }));

import { CoordinatorPage } from "./coordinator-page";

const assignment = (id: string, name: string) => ({ id, name, status: "idle" });
const catalog = (assignments: unknown[]) => ({
  workspace: { id: "ws", name: "Example workspace" },
  workflows: [],
  steps: [],
  repositories: [],
  assignments,
  profiles: [],
  executors: [],
});

afterEach(cleanup);

it("names the single orchestrator instead of offering a selector", () => {
  workspace.data = catalog([assignment("jeb", "Jeb")]);
  render(<CoordinatorPage workspaceId="ws" />);
  expect(screen.getByTestId("coordinator-name").textContent).toBe("Jeb");
  expect(screen.queryByTestId("coordinator-selector")).toBeNull();
  expect(screen.getByRole("link", { name: "Configure orchestrator" })).toBeTruthy();
});

it("keeps the selector for a legacy workspace with several orchestrators", () => {
  workspace.data = catalog([assignment("jeb", "Jeb"), assignment("ops", "Ops")]);
  render(<CoordinatorPage workspaceId="ws" />);
  expect(screen.getByTestId("coordinator-selector")).toBeTruthy();
  expect(screen.queryByTestId("coordinator-name")).toBeNull();
});
