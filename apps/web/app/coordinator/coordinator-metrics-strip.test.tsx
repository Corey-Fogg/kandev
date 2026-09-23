import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

const hook = vi.hoisted(() => ({ use: vi.fn() }));
vi.mock("@/hooks/domains/orchestration/use-coordinator-metrics", () => ({
  useCoordinatorMetrics: hook.use,
}));

import { CoordinatorMetricsStrip } from "./coordinator-metrics-strip";

const data = {
  days: 7,
  since: "2026-09-16T00:00:00Z",
  delegated: 12,
  completed: 8,
  failed: 1,
  truncated: true,
  success_rate: 0.89,
  merged_prs: null,
  cycle_time_samples: 8,
  cycle_time_median_hours: 3.5,
  cycle_time_p90_hours: 20.14,
  cost_usd: 4.2,
  cost_per_merged_pr_usd: null,
  unpriced_event_count: 2,
};
const tile = (id: string) => screen.getByTestId(`coordinator-metric-${id}`).textContent;

afterEach(() => {
  cleanup();
  hook.use.mockReset();
  window.localStorage.clear();
});

const expand = () => fireEvent.click(screen.getByTestId("coordinator-metrics-toggle"));

it("starts collapsed with a one-line summary and remembers expansion", () => {
  hook.use.mockReturnValue({ data, loading: false, refresh: vi.fn() });
  render(<CoordinatorMetricsStrip workspaceId="ws" orchestratorId="jeb" />);
  const toggle = screen.getByTestId("coordinator-metrics-toggle");
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  expect(toggle.textContent).toContain("8 completed, 1 failed");
  expect(screen.queryByTestId("coordinator-metric-completed")).toBeNull();
  expand();
  expect(toggle.getAttribute("aria-expanded")).toBe("true");
  expect(screen.getByTestId("coordinator-metric-completed")).toBeTruthy();
  cleanup();
  render(<CoordinatorMetricsStrip workspaceId="ws" orchestratorId="jeb" />);
  expect(screen.getByTestId("coordinator-metrics-toggle").getAttribute("aria-expanded")).toBe(
    "true",
  );
});

it("formats outcome tiles and switches the window", () => {
  hook.use.mockReturnValue({ data, loading: false, refresh: vi.fn() });
  render(<CoordinatorMetricsStrip workspaceId="ws" orchestratorId="jeb" />);
  expand();
  expect(tile("completed")).toContain("8");
  expect(tile("success")).toContain("89%");
  expect(tile("merged")).toContain("No data");
  expect(tile("cycle")).toContain("3.5 h");
  expect(tile("cycle")).toContain("90th percentile 20.1 h");
  expect(tile("cost")).toContain("at least $4.20");
  expect(screen.getByText("Some older tasks in this window were not counted.")).toBeTruthy();
  const month = screen.getByRole("button", { name: "30 days" });
  expect(month.getAttribute("aria-pressed")).toBe("false");
  fireEvent.click(month);
  expect(hook.use).toHaveBeenLastCalledWith("ws", "jeb", 30);
});

it("offers a retry when metrics are unavailable", () => {
  const refresh = vi.fn();
  hook.use.mockReturnValue({ error: new Error("down"), loading: false, refresh });
  render(<CoordinatorMetricsStrip workspaceId="ws" orchestratorId="jeb" />);
  expand();
  expect(screen.getByRole("alert").textContent).toContain("Outcome metrics are unavailable.");
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  expect(refresh).toHaveBeenCalled();
});
