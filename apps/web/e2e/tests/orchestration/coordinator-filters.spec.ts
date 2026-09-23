import { test, expect } from "../../fixtures/test-base";
import {
  addOrchestratorThroughSettings,
  resetWorkspaceOrchestrators,
} from "../../helpers/orchestration";

test("coordinator tasks filter with the sidebar's clauses and keep them per workspace", async ({
  testPage: page,
  backend,
  apiClient,
  seedData,
}) => {
  await backend.restart({
    KANDEV_FEATURES_ORCHESTRATION: "true",
    KANDEV_FEATURES_OFFICE: "false",
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  const ws = seedData.workspaceId;
  await resetWorkspaceOrchestrators(page.request, backend.baseUrl, ws);
  const placement = { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId };
  const guide = await apiClient.createTask(ws, "Draft the example guide", placement);
  const checklist = await apiClient.createTask(ws, "Review the sample checklist", placement);
  const archived = await apiClient.createTask(ws, "Archived example notes", placement);
  await apiClient.updateTaskState(checklist.id, "REVIEW");
  await apiClient.archiveTask(archived.id);
  const chief = await addOrchestratorThroughSettings(page, ws);
  await apiClient.updateTaskMetadata(guide.id, { orchestration_chief_id: chief });

  await page.goto(`/workspaces/${ws}/coordinator?orchestratorId=${chief}`);
  const row = (id: string) => page.getByTestId(`coordinator-task-${id}`);
  await expect(row(guide.id)).toBeVisible();
  await expect(row(checklist.id)).toBeVisible();
  await expect(row(archived.id)).toHaveCount(0);

  // A sidebar title clause narrows the list.
  const clauses = page.getByTestId("coordinator-filter-clauses");
  await clauses.getByRole("button", { name: "Add filter", exact: true }).click();
  const clause = clauses.getByTestId("filter-clause-row");
  await clause.getByTestId("filter-dimension-select").click();
  await page.getByRole("option", { name: "Title", exact: true }).click();
  await clause.getByTestId("filter-value-input").fill("checklist");
  await expect(row(checklist.id)).toBeVisible();
  await expect(row(guide.id)).toHaveCount(0);

  // Filters survive a reload of the same workspace.
  await page.reload();
  await expect(clauses.getByTestId("filter-value-input")).toHaveValue("checklist");
  await expect(row(checklist.id)).toBeVisible();
  await expect(row(guide.id)).toHaveCount(0);

  // Clauses combine with the coordinated scope.
  await page.getByRole("combobox", { name: "Task scope", exact: true }).click();
  await page.getByRole("option", { name: "Selected coordinator’s tasks", exact: true }).click();
  await expect(row(checklist.id)).toHaveCount(0);
  await clauses.getByRole("button", { name: "Clear filters", exact: true }).click();
  await expect(row(guide.id)).toBeVisible();
  await expect(row(checklist.id)).toHaveCount(0);
  await page.getByRole("combobox", { name: "Task scope", exact: true }).click();
  await page.getByRole("option", { name: "All workspace tasks", exact: true }).click();

  // The archived dimension reads archived tasks from the server.
  await clauses.getByRole("button", { name: "Add filter", exact: true }).click();
  await expect(clauses.getByTestId("filter-dimension-select")).toHaveText("Archived");
  await expect(row(archived.id)).toBeVisible();
  await expect(row(guide.id)).toHaveCount(0);
  await clauses.getByTestId("filter-clause-remove").click();
  await expect(row(archived.id)).toHaveCount(0);
  await expect(row(guide.id)).toBeVisible();

  // A sidebar view's clauses can be copied in.
  await page.getByTestId("coordinator-copy-sidebar-view").click();
  await expect(page.getByRole("option", { name: "All tasks", exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
});
