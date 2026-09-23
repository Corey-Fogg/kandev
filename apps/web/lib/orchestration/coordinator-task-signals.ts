import type { Task } from "@/lib/types/http";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

type Metadata = Task["metadata"];

export type SourceIssueChip = { tracker: "jira" | "linear"; key: string; url?: string };

export type CriterionStatus = "unverified" | "met" | "unmet";
export type Criterion = {
  id: string;
  text: string;
  status: CriterionStatus;
  evidence?: string;
  verified_at?: string;
};
export type TaskGoalSignal = { criteria: Criterion[]; met: number; total: number };

export type StallOutcome = "no_progress" | "never_started" | "orphaned";
export type StallSignal = { outcome: StallOutcome; stalledFor?: string; detectedAt: string };

const CRITERION_STATUSES: readonly string[] = ["unverified", "met", "unmet"];
const STALL_OUTCOMES: readonly string[] = ["no_progress", "never_started", "orphaned"];
const SETTLED_STATES: readonly string[] = ["COMPLETED", "CANCELLED"];

const record = (value: unknown): Record<string, unknown> | null =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
const text = (value: unknown): string | undefined =>
  typeof value === "string" && value.trim() ? value : undefined;

/** Only https links are rendered as links; anything else stays plain text. */
export function safeHttpsUrl(value: unknown): string | undefined {
  const raw = text(value);
  if (!raw) return undefined;
  try {
    return new URL(raw).protocol === "https:" ? raw : undefined;
  } catch {
    return undefined;
  }
}

/** The tracker issue a task was created from; Jira wins when both are present. */
export function sourceIssueFromMetadata(metadata: Metadata): SourceIssueChip | null {
  const m = record(metadata);
  if (!m) return null;
  const jira = text(m.jira_issue_key);
  if (jira) return { tracker: "jira", key: jira, url: safeHttpsUrl(m.jira_issue_url) };
  const linear = text(m.linear_issue_identifier);
  if (linear) return { tracker: "linear", key: linear, url: safeHttpsUrl(m.linear_issue_url) };
  return null;
}

function parseCriterion(value: unknown): Criterion | null {
  const c = record(value);
  const id = text(c?.id);
  const body = text(c?.text);
  if (!c || !id || !body) return null;
  const status = CRITERION_STATUSES.includes(String(c.status))
    ? (c.status as CriterionStatus)
    : "unverified";
  return { id, text: body, status, evidence: text(c.evidence), verified_at: text(c.verified_at) };
}

/** Acceptance criteria stored on a delegated task; malformed entries are skipped. */
export function goalFromMetadata(metadata: Metadata): TaskGoalSignal | null {
  const goal = record(record(metadata)?.orchestration_goal);
  if (!goal || !Array.isArray(goal.criteria)) return null;
  const criteria = goal.criteria.map(parseCriterion).filter((c): c is Criterion => c !== null);
  if (criteria.length === 0) return null;
  const met = criteria.filter((c) => c.status === "met").length;
  return { criteria, met, total: criteria.length };
}

/**
 * The task's last stall episode while it still applies: a settled task or any
 * activity after detection hides it.
 */
export function activeStall(task: Task): StallSignal | null {
  if (SETTLED_STATES.includes(task.state)) return null;
  const stall = record(record(task.metadata)?.orchestration_stall);
  if (!stall || !STALL_OUTCOMES.includes(String(stall.outcome))) return null;
  const detectedAt = text(stall.detected_at);
  const detected = parseStrictRfc3339Timestamp(detectedAt);
  if (!detectedAt || detected === null) return null;
  const activity = parseStrictRfc3339Timestamp(task.status_summary?.last_activity_at);
  if (activity !== null && activity > detected) return null;
  return {
    outcome: stall.outcome as StallOutcome,
    stalledFor: text(stall.stalled_for),
    detectedAt,
  };
}
