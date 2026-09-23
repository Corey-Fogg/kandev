import type { Page, Route } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  addOrchestratorThroughSettings,
  resetWorkspaceOrchestrators,
} from "../../helpers/orchestration";

type Proposal = Record<string, unknown> & { id: string; status: string };

/**
 * Serves the proposal endpoints from `proposals` so the card can be driven
 * without a coordinator turn; the proposal comment itself is a real comment.
 */
async function routeProposals(
  page: Page,
  proposals: Map<string, Proposal>,
  decide: (id: string, action: string, body: Record<string, unknown>) => Proposal,
) {
  await page.route(
    /\/api\/v1\/orchestration\/workspaces\/[^/]+\/orchestrators\/[^/]+\/proposals(\/.*)?(\?.*)?$/,
    async (route: Route) => {
      const url = new URL(route.request().url());
      const parts = url.pathname.split("/proposals")[1].split("/").filter(Boolean);
      if (route.request().method() === "GET" && parts.length === 0)
        return route.fulfill({ json: { proposals: [...proposals.values()] } });
      const proposal = proposals.get(parts[0]);
      if (!proposal) return route.fulfill({ status: 404, json: { error: "proposal_not_found" } });
      if (route.request().method() === "GET") return route.fulfill({ json: proposal });
      const next = decide(parts[0], parts[1], route.request().postDataJSON() ?? {});
      proposals.set(next.id, next);
      return route.fulfill({ json: { proposal: next, task_id: next.task_id, duplicate: false } });
    },
  );
}

test("a task proposal renders as a card that can be edited, approved or dismissed", async ({
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
  const chief = await addOrchestratorThroughSettings(page, ws);
  const conversation = await (
    await page.request.post(
      `${backend.baseUrl}/api/v1/orchestration/workspaces/${ws}/orchestrators/${chief}/conversation`,
    )
  ).json();
  const seedProposalComment = async (title: string) =>
    (
      await apiClient.seedComment({
        taskId: conversation.task_id,
        authorType: "agent",
        authorId: chief,
        source: "proposal",
        body: `**Proposed task:** ${title}\n\nApprove, edit or dismiss it in the Coordinator view.`,
      })
    ).comment_id;
  const approveId = await seedProposalComment("Write the example setup guide");
  const dismissId = await seedProposalComment("Rename the sample checklist");
  const created = await apiClient.createTask(ws, "Write the example setup guide", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const proposal = (id: string, title: string): Proposal => ({
    id,
    orchestrator_id: chief,
    workspace_id: ws,
    conversation_task_id: conversation.task_id,
    status: "pending",
    spec: {
      title,
      workflow_id: seedData.workflowId,
      source: { tracker: "jira", key: "EX-12", url: "https://example.atlassian.net/browse/EX-12" },
      acceptance_criteria: ["The guide lists every example step"],
    },
    edited: false,
    task_id: "",
    duplicate: false,
    dismiss_reason: "",
    decided_by: "",
    created_at: new Date().toISOString(),
    decided_at: null,
  });
  const proposals = new Map<string, Proposal>([
    [approveId, proposal(approveId, "Write the example setup guide")],
    [dismissId, proposal(dismissId, "Rename the sample checklist")],
  ]);
  await routeProposals(page, proposals, (id, action, body) => {
    const current = proposals.get(id)!;
    if (action === "dismiss")
      return { ...current, status: "dismissed", dismiss_reason: String(body.reason ?? "") };
    const edits = (body.edits ?? {}) as Record<string, unknown>;
    const edited = Object.keys(edits).length > 0;
    return {
      ...current,
      status: "approved",
      task_id: created.id,
      edited,
      final_spec: edited ? { ...(current.spec as object), ...edits } : null,
    };
  });

  await page.goto(`/workspaces/${ws}/coordinator?orchestratorId=${chief}`);
  const chat = page.getByTestId("orchestrator-conversation");
  const approveCard = chat.locator(`[data-proposal-id="${approveId}"]`);
  const dismissCard = chat.locator(`[data-proposal-id="${dismissId}"]`);
  await expect(approveCard).toBeVisible();
  await expect(
    approveCard.getByRole("heading", { name: "Write the example setup guide" }),
  ).toBeVisible();
  await expect(approveCard.getByRole("link", { name: "Open Jira issue EX-12" })).toBeVisible();
  await expect(approveCard.getByText("The guide lists every example step")).toBeVisible();
  await expect(chat.getByTestId("proposals-pending-banner")).toHaveText(
    "2 task proposals await your decision",
  );

  // Edit, then approve with only the changed title.
  await approveCard.getByRole("button", { name: "Edit", exact: true }).click();
  await approveCard.getByTestId("proposal-edit-title").fill("Write the short setup guide");
  const approved = waitForHttp(page, "POST", new RegExp(`/proposals/${approveId}/approve$`));
  await approveCard.getByRole("button", { name: "Approve with changes", exact: true }).click();
  expect((await approved).request().postDataJSON()).toEqual({
    edits: { title: "Write the short setup guide" },
  });
  await expect(approveCard).toHaveAttribute("data-status", "approved");
  await expect(
    approveCard.getByRole("heading", { name: "Write the short setup guide" }),
  ).toBeVisible();
  await expect(approveCard.getByText("Approved with your changes.")).toBeVisible();
  await expect(approveCard.getByRole("link", { name: "Open task", exact: true })).toHaveAttribute(
    "href",
    `/t/${created.id}`,
  );
  await expect(chat.getByTestId("proposals-pending-banner")).toHaveText(
    "1 task proposal awaits your decision",
  );

  // Dismiss with a reason.
  await dismissCard.getByRole("button", { name: "Dismiss", exact: true }).click();
  await dismissCard.getByTestId("proposal-dismiss-reason").fill("Not needed this week");
  const dismissed = waitForHttp(page, "POST", new RegExp(`/proposals/${dismissId}/dismiss$`));
  await dismissCard.getByRole("button", { name: "Confirm dismissal", exact: true }).click();
  expect((await dismissed).request().postDataJSON()).toEqual({ reason: "Not needed this week" });
  await expect(dismissCard).toHaveAttribute("data-status", "dismissed");
  await expect(dismissCard.getByText("Reason: Not needed this week")).toBeVisible();
  await expect(chat.getByTestId("proposals-pending-banner")).toHaveCount(0);
});
