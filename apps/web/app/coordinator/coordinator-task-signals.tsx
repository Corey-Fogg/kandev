import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import type { Task } from "@/lib/types/http";
import {
  activeStall,
  goalFromMetadata,
  sourceIssueFromMetadata,
  type SourceIssueChip as SourceIssue,
  type TaskGoalSignal,
} from "@/lib/orchestration/coordinator-task-signals";
import { formatStallDuration, parseGoDuration } from "@/lib/orchestration/stall-duration";

// Catalog keys, not copy: each resolves through `t()` at render.
const TRACKER_KEYS = {
  jira: "orchestration:tracker_jira",
  linear: "orchestration:tracker_linear",
} as const;
const STALL_KEYS = {
  no_progress: "orchestration:stall_no_progress",
  never_started: "orchestration:stall_never_started",
  orphaned: "orchestration:stall_orphaned",
} as const;
const CRITERION_KEYS = {
  unverified: "orchestration:criterionStatus_unverified",
  met: "orchestration:criterionStatus_met",
  unmet: "orchestration:criterionStatus_unmet",
} as const;
const PR_STATE_KEYS: Record<string, string> = {
  open: "orchestration:pullRequestState_open",
  merged: "orchestration:pullRequestState_merged",
  closed: "orchestration:pullRequestState_closed",
  draft: "orchestration:pullRequestState_draft",
};

const CHIP = "max-md:min-h-11 max-md:px-3";
const LINK_CHIP = `${CHIP} cursor-pointer`;

export function safeExternalPR(value?: string) {
  return value && /^https?:\/\//i.test(value) ? value : undefined;
}

/** The tracker key of the issue a task came from, linked when the link is https. */
export function SourceIssueChip({ source }: { source: SourceIssue }) {
  const { t } = useTranslation();
  const tracker = t(TRACKER_KEYS[source.tracker]);
  const label = `${tracker} ${source.key}`;
  if (!source.url)
    return (
      <Badge variant="outline" className={CHIP} data-testid="source-issue-chip">
        {label}
      </Badge>
    );
  return (
    <Badge variant="outline" asChild className={LINK_CHIP}>
      <a
        href={source.url}
        target="_blank"
        rel="noreferrer"
        aria-label={t("orchestration:sourceIssueLink", { key: source.key, tracker })}
        data-testid="source-issue-chip"
      >
        {label}
      </a>
    </Badge>
  );
}

export function PullRequestChip({ task }: { task: Task }) {
  const { t } = useTranslation();
  const pr = task.status_summary?.pull_request;
  if (!pr || (pr.number === undefined && !pr.state)) return null;
  const href = safeExternalPR(pr.url);
  const content = (
    <>
      {pr.number !== undefined && (
        <span>{t("orchestration:pullRequest", { number: pr.number })}</span>
      )}
      {pr.state && (
        <span className="text-muted-foreground">
          {PR_STATE_KEYS[pr.state.toLowerCase()]
            ? t(PR_STATE_KEYS[pr.state.toLowerCase()])
            : pr.state}
        </span>
      )}
    </>
  );
  if (!href)
    return (
      <Badge variant="outline" className={CHIP} data-testid="pull-request-chip">
        {content}
      </Badge>
    );
  return (
    <Badge variant="outline" asChild className={LINK_CHIP}>
      <a href={href} target="_blank" rel="noreferrer" data-testid="pull-request-chip">
        {content}
      </a>
    </Badge>
  );
}

/** The stall's outcome, with how long the task was stalled shown beside it. */
export function StallBadge({ task }: { task: Task }) {
  const { t, i18n } = useTranslation();
  const stall = activeStall(task);
  if (!stall) return null;
  const seconds = parseGoDuration(stall.stalledFor);
  return (
    <Badge
      variant="outline"
      className={`${CHIP} gap-1.5 border-destructive text-destructive`}
      data-testid="stall-badge"
    >
      <span>{t(STALL_KEYS[stall.outcome])}</span>
      {seconds !== null && (
        <span className="font-normal" data-testid="stall-duration">
          {t("orchestration:stallFor", {
            duration: formatStallDuration(seconds, i18n.resolvedLanguage ?? i18n.language),
          })}
        </span>
      )}
    </Badge>
  );
}

/** Criteria progress; expanding it shows each criterion with its recorded evidence. */
export function CriteriaChip({ goal }: { goal: TaskGoalSignal }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const listId = useId();
  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={`h-5 px-2 text-[0.625rem] cursor-pointer ${CHIP}`}
        aria-expanded={open}
        aria-controls={listId}
        onClick={() => setOpen(!open)}
        data-testid="criteria-chip"
      >
        {t("orchestration:criteriaProgress", { met: goal.met, total: goal.total })}
      </Button>
      {open && (
        <ul
          id={listId}
          className="w-full space-y-1 rounded-md border bg-muted/30 p-2 text-foreground"
          aria-label={t("orchestration:criteriaHeading")}
          data-testid="criteria-list"
        >
          {goal.criteria.map((criterion) => (
            <li key={criterion.id} className="space-y-0.5">
              <div className="flex flex-wrap items-start gap-2">
                <Badge variant={criterion.status === "met" ? "secondary" : "outline"}>
                  {t(CRITERION_KEYS[criterion.status])}
                </Badge>
                <span className="min-w-0 flex-1 break-words">{criterion.text}</span>
              </div>
              {criterion.evidence && (
                <p className="break-words text-muted-foreground">
                  {t("orchestration:criterionEvidence", { evidence: criterion.evidence })}
                </p>
              )}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

/** Coordinator row signals: source issue, pull request, stall and acceptance criteria. */
export function CoordinatorTaskSignals({ task }: { task: Task }) {
  const source = sourceIssueFromMetadata(task.metadata);
  const goal = goalFromMetadata(task.metadata);
  return (
    <>
      {source && <SourceIssueChip source={source} />}
      <PullRequestChip task={task} />
      <StallBadge task={task} />
      {goal && <CriteriaChip goal={goal} />}
    </>
  );
}
