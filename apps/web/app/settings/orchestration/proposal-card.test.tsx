import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { TaskComment } from "@/app/office/tasks/[id]/types";
import type { TaskProposal } from "@/lib/api/domains/orchestration-proposals-api";
import { ProposalCatalogContext } from "@/lib/orchestration/proposal-catalog";

const api = vi.hoisted(() => ({ approve: vi.fn(), dismiss: vi.fn() }));
vi.mock("@/lib/api/domains/orchestration-proposals-api", () => ({
  approveTaskProposal: api.approve,
  dismissTaskProposal: api.dismiss,
}));
vi.mock("@/components/task/simple/markdown-comment", () => ({
  MarkdownComment: ({ content }: { content: string }) => <p>{content}</p>,
}));

import { ProposalCard } from "./proposal-card";

const APPROVE = "proposal-approve";
const LEXER = "Fix the lexer";

const comment = { id: "p1", content: "**Proposed task:** Fix parser" } as TaskComment;
const base: TaskProposal = {
  id: "p1",
  orchestrator_id: "jeb",
  workspace_id: "ws",
  conversation_task_id: "conv",
  status: "pending",
  spec: {
    title: "Fix the parser",
    workflow_id: "wf",
    source: { tracker: "jira", key: "ABC-1", url: "https://example.atlassian.net/browse/ABC-1" },
    acceptance_criteria: ["Parser tests pass"],
  },
  edited: false,
  task_id: "",
  duplicate: false,
  dismiss_reason: "",
  decided_by: "",
  created_at: "2026-09-23T12:00:00Z",
  decided_at: null,
};
const catalog = {
  workflows: [{ id: "wf", name: "Delivery" }],
  steps: [],
  repositories: [],
  profiles: [],
};

function renderCard(proposal: TaskProposal | undefined, onChange = vi.fn()) {
  render(
    <ProposalCatalogContext.Provider value={catalog}>
      <ProposalCard
        comment={comment}
        proposal={proposal}
        workspaceId="ws"
        orchestratorId="jeb"
        onChange={onChange}
      />
    </ProposalCatalogContext.Provider>,
  );
  return onChange;
}

afterEach(() => {
  cleanup();
  api.approve.mockReset();
  api.dismiss.mockReset();
});

it("shows the proposed task with catalog names, source and criteria", () => {
  renderCard(base);
  expect(screen.getByRole("heading", { name: "Fix the parser" })).toBeTruthy();
  expect(screen.getByText("Delivery")).toBeTruthy();
  expect(screen.getByRole("link", { name: "Open Jira issue ABC-1" })).toBeTruthy();
  expect(screen.getByText("Parser tests pass")).toBeTruthy();
  expect(screen.getByText("Awaiting your decision")).toBeTruthy();
});

it("sends one approval for a double click and hands back the result", async () => {
  let finish!: (value: unknown) => void;
  api.approve.mockReturnValue(new Promise((resolve) => (finish = resolve)));
  const onChange = renderCard(base);
  const approve = screen.getByTestId(APPROVE);
  fireEvent.click(approve);
  fireEvent.click(approve);
  expect(api.approve).toHaveBeenCalledTimes(1);
  expect(api.approve).toHaveBeenCalledWith("ws", "jeb", "p1", undefined);
  const approved = { ...base, status: "approved", task_id: "t1" };
  finish({ proposal: approved, task_id: "t1", duplicate: false });
  await waitFor(() => expect(onChange).toHaveBeenCalledWith(approved));
});

it("dismisses with an optional reason", async () => {
  api.dismiss.mockResolvedValue({ proposal: { ...base, status: "dismissed" } });
  const onChange = renderCard(base);
  fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
  fireEvent.change(screen.getByTestId("proposal-dismiss-reason"), {
    target: { value: "Already fixed" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Confirm dismissal" }));
  await waitFor(() => expect(onChange).toHaveBeenCalled());
  expect(api.dismiss).toHaveBeenCalledWith("ws", "jeb", "p1", "Already fixed");
});

it("replaces the card state with the proposal a 409 returns", async () => {
  const dismissed = { ...base, status: "dismissed" as const, dismiss_reason: "No" };
  api.approve.mockRejectedValue(
    new ApiError("conflict", 409, { error: "proposal_already_decided", proposal: dismissed }),
  );
  const onChange = renderCard(base);
  fireEvent.click(screen.getByTestId(APPROVE));
  await waitFor(() => expect(onChange).toHaveBeenCalledWith(dismissed));
  expect(screen.queryByRole("alert")).toBeNull();
});

it("explains a forbidden decision and other failures", async () => {
  api.approve.mockRejectedValueOnce(new ApiError("forbidden", 403, null));
  renderCard(base);
  fireEvent.click(screen.getByTestId(APPROVE));
  expect((await screen.findByRole("alert")).textContent).toContain("permission");
  api.approve.mockRejectedValueOnce(new ApiError("bad", 422, { error: "title too long" }));
  fireEvent.click(screen.getByTestId(APPROVE));
  await waitFor(() =>
    expect(screen.getByRole("alert").textContent).toBe(
      "The decision could not be saved: title too long",
    ),
  );
});

it("links an approved proposal to its task with edit and duplicate notes", () => {
  renderCard({
    ...base,
    status: "approved",
    task_id: "t1",
    edited: true,
    duplicate: true,
    final_spec: { ...base.spec, title: LEXER },
  });
  expect(screen.getByRole("link", { name: "Open task" }).getAttribute("href")).toBe("/t/t1");
  expect(screen.getByRole("heading", { name: LEXER })).toBeTruthy();
  expect(screen.getByText("Approved with your changes.")).toBeTruthy();
  expect(screen.getByText(/already existed/)).toBeTruthy();
  expect(screen.queryByTestId(APPROVE)).toBeNull();
});

it("approves an edited proposal with only the changed fields", async () => {
  api.approve.mockResolvedValue({ proposal: { ...base, status: "approved" } });
  renderCard(base);
  fireEvent.click(screen.getByRole("button", { name: "Edit" }));
  fireEvent.change(screen.getByTestId("proposal-edit-title"), {
    target: { value: LEXER },
  });
  fireEvent.click(screen.getByRole("button", { name: "Approve with changes" }));
  await waitFor(() =>
    expect(api.approve).toHaveBeenCalledWith("ws", "jeb", "p1", { title: LEXER }),
  );
});

it("falls back to the comment body until the proposal loads", () => {
  renderCard(undefined);
  expect(screen.getByText("**Proposed task:** Fix parser")).toBeTruthy();
});
