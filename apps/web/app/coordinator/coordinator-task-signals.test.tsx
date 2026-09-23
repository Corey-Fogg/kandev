import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { Task } from "@/lib/types/http";
import { CoordinatorTaskSignals } from "./coordinator-task-signals";

afterEach(cleanup);

const task = {
  id: "t",
  state: "IN_PROGRESS",
  metadata: {
    jira_issue_key: "ABC-1",
    jira_issue_url: "https://example.atlassian.net/browse/ABC-1",
    orchestration_stall: { outcome: "never_started", detected_at: "2026-09-23T12:00:00Z" },
    orchestration_goal: {
      criteria: [
        { id: "c1", text: "Parser tests", status: "met", evidence: "12 tests pass" },
        { id: "c2", text: "Docs", status: "unverified" },
      ],
    },
  },
  status_summary: {
    revision: 1,
    updated_at: "2026-09-23T12:00:00Z",
    pull_request: { number: 7, state: "merged", url: "https://github.com/example/repo/pull/7" },
  },
} as unknown as Task;

it("renders source, pull request, stall and criteria signals", () => {
  render(<CoordinatorTaskSignals task={task} />);
  const source = screen.getByRole("link", { name: "Open Jira issue ABC-1" });
  expect(source.getAttribute("href")).toBe("https://example.atlassian.net/browse/ABC-1");
  const pr = screen.getByTestId("pull-request-chip");
  expect(pr.textContent).toBe("PR #7Merged");
  expect(pr.getAttribute("href")).toBe("https://github.com/example/repo/pull/7");
  expect(screen.getByTestId("stall-badge").textContent).toBe("Stalled: never started");
  const criteria = screen.getByRole("button", { name: "Criteria 1/2 met" });
  expect(criteria.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(criteria);
  expect(criteria.getAttribute("aria-expanded")).toBe("true");
  expect(screen.getByTestId("criteria-list").textContent).toContain("Evidence: 12 tests pass");
});

it("renders nothing for a task without signals", () => {
  const { container } = render(
    <CoordinatorTaskSignals task={{ id: "t", state: "TODO", metadata: null } as unknown as Task} />,
  );
  expect(container.textContent).toBe("");
});

it("shows how long a task was stalled as visible, localized text", () => {
  const stalled = {
    ...task,
    metadata: {
      orchestration_stall: {
        outcome: "no_progress",
        stalled_for: "2h5m0s",
        detected_at: "2026-09-23T12:00:00Z",
      },
    },
  } as unknown as Task;
  render(<CoordinatorTaskSignals task={stalled} />);
  expect(screen.getByTestId("stall-duration").textContent).toBe("Stalled for 2 hours, 5 minutes");
  expect(screen.getByTestId("stall-badge").getAttribute("title")).toBeNull();
});
