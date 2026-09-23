import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  addOrchestratorThroughSettings,
  resetWorkspaceOrchestrators,
} from "../../helpers/orchestration";

test("the workspace orchestrator can be renamed and is listed by name in both navigation layouts", async ({
  testPage: page,
  backend,
  seedData,
}) => {
  await backend.restart({
    KANDEV_FEATURES_ORCHESTRATION: "true",
    KANDEV_FEATURES_OFFICE: "false",
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  const ws = seedData.workspaceId;
  const base = `${backend.baseUrl}/api/v1/orchestration/workspaces/${ws}/orchestrators`;
  await resetWorkspaceOrchestrators(page.request, backend.baseUrl, ws);
  const id = await addOrchestratorThroughSettings(page, ws);
  const created = await (await page.request.get(`${base}/${id}`)).json();
  expect(created.display_name).toBe("");
  expect(created.name).toBe(created.role_name);

  // Rename through the settings editor; identity and behavior save through PATCH.
  const displayName = page.getByTestId("orchestrator-display-name");
  await expect(displayName).toHaveAttribute("placeholder", created.role_name);
  await displayName.fill("Jeb");
  await page.getByTestId("orchestrator-ask_before_create").click();
  const patched = waitForHttp(page, "PATCH", new RegExp(`/orchestrators/${id}$`));
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  const response = await patched;
  expect(response.status()).toBe(200);
  expect(response.request().postDataJSON()).toEqual({
    display_name: "Jeb",
    ask_before_create: true,
  });
  await expect(page.getByRole("heading", { name: "Jeb", exact: true })).toBeVisible();
  const renamed = await (await page.request.get(`${base}/${id}`)).json();
  expect(renamed).toMatchObject({
    name: "Jeb",
    display_name: "Jeb",
    role_name: created.role_name,
    ask_before_create: true,
    auto_comment_source: true,
    auto_move_source_done: false,
  });

  // The sidebar lists the orchestrator by its name and opens it in the Coordinator view.
  const navEntry = page.getByTestId(`workspace-coordinator-link-${id}`);
  await page.goto(`/?workspaceId=${ws}`);
  await expect(navEntry).toHaveText("Jeb");
  await expect(page.getByTestId("workspace-coordinator-link")).toHaveCount(0);
  await navEntry.click();
  await expect(page).toHaveURL(new RegExp(`/workspaces/${ws}/coordinator\\?orchestratorId=${id}`));
  await expect(page.getByTestId("coordinator-name")).toHaveText("Jeb");
  await expect(page.getByTestId("coordinator-selector")).toHaveCount(0);
  await expect(
    page.getByTestId("orchestrator-conversation").getByRole("heading", {
      name: "Conversation with Jeb",
      exact: true,
    }),
  ).toBeVisible();

  // The phone navigation sheet lists the same entry and closes when it navigates.
  await page.goto(`/?workspaceId=${ws}`);
  await page.setViewportSize({ width: 393, height: 852 });
  await page.getByTestId("app-nav-trigger").click();
  const sheet = page.getByTestId("app-nav-sheet");
  await expect(sheet.getByTestId(`workspace-coordinator-link-${id}`)).toHaveText("Jeb");
  await sheet.getByTestId(`workspace-coordinator-link-${id}`).click();
  await expect(sheet).not.toBeVisible();
  await expect(page).toHaveURL(new RegExp(`orchestratorId=${id}`));

  // Clearing the name falls back to the role name everywhere.
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto(`/settings/workspaces/${ws}/orchestration/${id}`);
  await page.getByTestId("orchestrator-display-name").fill("");
  const cleared = waitForHttp(page, "PATCH", new RegExp(`/orchestrators/${id}$`));
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  expect((await cleared).request().postDataJSON()).toEqual({ display_name: "" });
  await expect(page.getByRole("heading", { name: created.role_name, exact: true })).toBeVisible();
  await page.goto(`/?workspaceId=${ws}`);
  await expect(navEntry).toHaveText(created.role_name);

  // Only one orchestrator per workspace: the settings list offers no second one.
  await page.goto(`/settings/workspaces/${ws}/orchestration`);
  await expect(page.getByTestId("orchestrator-card")).toHaveCount(1);
  await expect(page.getByRole("link", { name: "Add orchestrator", exact: true })).toHaveCount(0);
  const second = await page.request.post(base, { data: renamed });
  expect(second.status()).toBe(409);
});
