import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { CoordinatorWorkspace } from "@/hooks/domains/orchestration/use-coordinator-workspace";
import type { FilterClause } from "@/lib/state/slices/ui/sidebar-view-types";

const store = vi.hoisted(() => ({
  workspaces: { activeId: "ws" },
  sidebarViewsByWorkspace: {
    ws: {
      views: [
        {
          id: "mine",
          name: "My review",
          filters: [{ id: "old", dimension: "hasPR", op: "is", value: true }],
          sort: { key: "state", direction: "asc" },
          group: "none",
          collapsedGroups: [],
        },
      ],
      activeViewId: "mine",
      draft: null,
      syncError: null,
    },
  },
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (s: typeof store) => unknown) => select(store),
}));

import { CoordinatorFilterClauses } from "./coordinator-filter-clauses";

const catalog = {
  workspace: { id: "ws", name: "Example", office_workflow_id: "office" },
  workflows: [
    { id: "wf", name: "Delivery" },
    { id: "office", name: "Office" },
  ],
  steps: [],
  repositories: [],
} as unknown as CoordinatorWorkspace;

function renderClauses(clauses: FilterClause[], onChange = vi.fn()) {
  render(
    <CoordinatorFilterClauses
      catalog={catalog}
      tasks={[]}
      clauses={clauses}
      onChange={onChange}
      mobile={false}
    />,
  );
  return onChange;
}

afterEach(cleanup);

it("adds a clause with the sidebar's first dimension defaults", () => {
  const onChange = renderClauses([]);
  expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Add filter" }));
  const [added] = onChange.mock.calls[0][0] as FilterClause[];
  expect(added).toMatchObject({ dimension: "archived", op: "is", value: true });
  expect(added.id).toBeTruthy();
});

it("removes and clears clauses", () => {
  const clause: FilterClause = { id: "c1", dimension: "hasPR", op: "is", value: true };
  const onChange = renderClauses([clause]);
  expect(screen.getAllByTestId("filter-clause-row")).toHaveLength(1);
  fireEvent.click(screen.getByTestId("filter-clause-remove"));
  expect(onChange).toHaveBeenLastCalledWith([]);
  fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
  expect(onChange).toHaveBeenLastCalledWith([]);
});

it("offers the workspace's sidebar views to copy from", () => {
  renderClauses([]);
  expect(screen.getByTestId("coordinator-copy-sidebar-view").textContent).toContain(
    "Choose a view",
  );
});
