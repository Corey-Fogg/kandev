import { describe, expect, it } from "vitest";
import type { Task } from "@/lib/types/http";
import {
  activeStall,
  goalFromMetadata,
  safeHttpsUrl,
  sourceIssueFromMetadata,
} from "./coordinator-task-signals";

const DETECTED = "2026-09-23T12:00:00.123Z";
const task = (patch: Partial<Task>) =>
  ({ id: "t", state: "IN_PROGRESS", metadata: {}, ...patch }) as Task;
const stalled = (patch: Partial<Task> = {}) =>
  task({
    metadata: {
      orchestration_stall: { outcome: "no_progress", stalled_for: "5m0s", detected_at: DETECTED },
    },
    ...patch,
  });

describe("source issue chip", () => {
  it("prefers Jira over Linear and keeps only https links", () => {
    expect(
      sourceIssueFromMetadata({
        jira_issue_key: "ABC-1",
        jira_issue_url: "https://example.atlassian.net/browse/ABC-1",
        linear_issue_identifier: "ENG-2",
      }),
    ).toEqual({
      tracker: "jira",
      key: "ABC-1",
      url: "https://example.atlassian.net/browse/ABC-1",
    });
    expect(
      sourceIssueFromMetadata({
        linear_issue_identifier: "ENG-2",
        linear_issue_url: "http://linear.example/ENG-2",
      }),
    ).toEqual({ tracker: "linear", key: "ENG-2", url: undefined });
    expect(sourceIssueFromMetadata({ jira_issue_key: "" })).toBeNull();
    expect(sourceIssueFromMetadata(null)).toBeNull();
    expect(safeHttpsUrl("javascript:alert(1)")).toBeUndefined();
    expect(safeHttpsUrl("not a url")).toBeUndefined();
  });
});

describe("acceptance criteria", () => {
  it("parses tolerantly and counts met criteria", () => {
    const goal = goalFromMetadata({
      orchestration_goal: {
        criteria: [
          { id: "c1", text: "Tests pass", status: "met", evidence: "CI green" },
          { id: "c2", text: "Docs updated", status: "strange" },
          { id: "c3", text: "" },
          "garbage",
          { id: "c4", text: "Reviewed", status: "unmet", evidence: "No review yet" },
        ],
      },
    });
    expect(goal?.total).toBe(3);
    expect(goal?.met).toBe(1);
    expect(goal?.criteria.map((c) => c.status)).toEqual(["met", "unverified", "unmet"]);
    expect(goal?.criteria[0].evidence).toBe("CI green");
  });

  it("returns null when the goal is absent or has no valid criteria", () => {
    expect(goalFromMetadata({})).toBeNull();
    expect(goalFromMetadata({ orchestration_goal: null })).toBeNull();
    expect(goalFromMetadata({ orchestration_goal: { criteria: "x" } })).toBeNull();
    expect(goalFromMetadata({ orchestration_goal: { criteria: [{ id: "c1" }] } })).toBeNull();
  });
});

describe("stall badge", () => {
  it("shows the last stall until newer activity or a settled state", () => {
    expect(activeStall(stalled())).toEqual({
      outcome: "no_progress",
      stalledFor: "5m0s",
      detectedAt: DETECTED,
    });
    const older = { revision: 1, updated_at: DETECTED, last_activity_at: "2026-09-23T11:00:00Z" };
    expect(activeStall(stalled({ status_summary: older }))).not.toBeNull();
    const newer = { revision: 2, updated_at: DETECTED, last_activity_at: "2026-09-23T12:00:01Z" };
    expect(activeStall(stalled({ status_summary: newer }))).toBeNull();
    expect(activeStall(stalled({ state: "COMPLETED" }))).toBeNull();
    expect(activeStall(stalled({ state: "CANCELLED" }))).toBeNull();
  });

  it("ignores unknown outcomes and malformed detection times", () => {
    expect(
      activeStall(
        task({ metadata: { orchestration_stall: { outcome: "x", detected_at: DETECTED } } }),
      ),
    ).toBeNull();
    expect(
      activeStall(
        task({ metadata: { orchestration_stall: { outcome: "orphaned", detected_at: "soon" } } }),
      ),
    ).toBeNull();
  });
});
