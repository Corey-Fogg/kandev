import { expect, type APIRequestContext, type Page } from "@playwright/test";

/** The worker's task reset does not remove workspace orchestrator registrations. */
export async function resetWorkspaceOrchestrators(
  request: APIRequestContext,
  baseUrl: string,
  workspaceId: string,
): Promise<void> {
  const url = `${baseUrl}/api/v1/orchestration/workspaces/${workspaceId}/orchestrators`;
  const response = await request.get(url);
  expect(response.ok()).toBeTruthy();
  const { orchestrators } = (await response.json()) as { orchestrators: { id: string }[] };
  for (const orchestrator of orchestrators) {
    const deleted = await request.delete(`${url}/${orchestrator.id}`);
    expect(deleted.ok()).toBeTruthy();
  }
}

/** Adds the workspace's orchestrator through the settings form and returns its id. */
export async function addOrchestratorThroughSettings(page: Page, workspaceId: string) {
  await page.goto(`/settings/workspaces/${workspaceId}/orchestration/new`);
  await page.getByTestId("orchestrator-profile").click();
  await page.getByRole("option").first().click();
  await page.getByTestId("persona-executor-profile").click();
  await page.getByRole("option").filter({ hasNotText: "Inherit" }).first().click();
  await page.getByRole("button", { name: "Add orchestrator", exact: true }).click();
  await expect(page).not.toHaveURL(/\/new$/);
  return new URL(page.url()).pathname.split("/").pop()!;
}
