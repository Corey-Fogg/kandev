import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { OrchestratorTargetField } from "./orchestrator-target-field";

const feature = vi.hoisted(() => ({ orchestration: true }));
const read = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => feature.orchestration,
}));
vi.mock("@/lib/orchestration/orchestrator-list-cache", () => ({
  readWorkspaceOrchestrators: read,
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  feature.orchestration = true;
});

it("hides the target while orchestration is off and nothing is selected", () => {
  feature.orchestration = false;
  const { container } = render(
    <OrchestratorTargetField workspaceId="ws" value="" onChange={vi.fn()} />,
  );
  expect(container.textContent).toBe("");
  expect(read).not.toHaveBeenCalled();
});

it("keeps an existing orchestrator target visible while orchestration is off", () => {
  feature.orchestration = false;
  render(<OrchestratorTargetField workspaceId="ws" value="o" onChange={vi.fn()} />);
  expect(screen.getByTestId("automation-target")).toBeTruthy();
  expect(read).not.toHaveBeenCalled();
});

it("reads the workspace orchestrators through the shared cache", async () => {
  read.mockResolvedValue({ orchestrators: [{ id: "o", name: "Chief" }] });
  render(<OrchestratorTargetField workspaceId="ws" value="o" onChange={vi.fn()} />);
  await waitFor(() =>
    expect(screen.getByTestId("automation-target").textContent).toContain("Chief"),
  );
  expect(read).toHaveBeenCalledWith("ws");
});

it("reports a failed read", async () => {
  read.mockRejectedValue(new Error("offline"));
  render(<OrchestratorTargetField workspaceId="ws" value="" onChange={vi.fn()} />);
  await waitFor(() => expect(screen.getByRole("alert").textContent).toContain("offline"));
});
