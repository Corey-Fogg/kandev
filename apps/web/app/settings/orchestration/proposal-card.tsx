import { useContext, useId, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { MarkdownComment } from "@/components/task/simple/markdown-comment";
import { SourceIssueChip } from "@/app/coordinator/coordinator-task-signals";
import type { TaskComment } from "@/app/office/tasks/[id]/types";
import type { ProposalSpec, TaskProposal } from "@/lib/api/domains/orchestration-proposals-api";
import { safeHttpsUrl } from "@/lib/orchestration/coordinator-task-signals";
import { ProposalCatalogContext, proposalNames } from "@/lib/orchestration/proposal-catalog";
import { ProposalActions } from "./proposal-actions";

// Catalog keys, not copy: each resolves through `t()` at render.
const STATUS_KEYS = {
  pending: "orchestration:proposalStatus_pending",
  approving: "orchestration:proposalStatus_approving",
  approved: "orchestration:proposalStatus_approved",
  dismissed: "orchestration:proposalStatus_dismissed",
} as const;
const MODE_KEYS: Record<string, string> = {
  execute: "orchestration:proposalExecutionMode_execute",
  design: "orchestration:proposalExecutionMode_design",
};

type CardProps = {
  comment: TaskComment;
  proposal?: TaskProposal;
  unavailable?: boolean;
  workspaceId: string;
  orchestratorId: string;
  onChange: (proposal: TaskProposal) => void;
};

function SpecDetails({ spec }: { spec: ProposalSpec }) {
  const { t } = useTranslation();
  const names = proposalNames(useContext(ProposalCatalogContext), spec);
  const mode = spec.execution_mode ? MODE_KEYS[spec.execution_mode] : undefined;
  const rows = [
    { label: t("orchestration:proposalFieldWorkflow"), value: names.workflow },
    { label: t("orchestration:proposalFieldStep"), value: names.step },
    { label: t("orchestration:proposalFieldRepository"), value: names.repository },
    { label: t("orchestration:proposalFieldAssignee"), value: names.assignee },
    { label: t("orchestration:proposalFieldExecutionMode"), value: mode ? t(mode) : "" },
  ].filter((row) => row.value);
  const source = spec.source ? { ...spec.source, url: safeHttpsUrl(spec.source.url) } : undefined;
  return (
    <>
      {spec.description && (
        <details className="text-sm">
          <summary className="cursor-pointer text-muted-foreground max-md:min-h-11">
            {t("orchestration:proposalFieldDescription")}
          </summary>
          <MarkdownComment content={spec.description} />
        </details>
      )}
      {rows.length > 0 && (
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
          {rows.map((row) => (
            <div key={row.label} className="contents">
              <dt className="text-muted-foreground">{row.label}</dt>
              <dd className="min-w-0 break-words">{row.value}</dd>
            </div>
          ))}
        </dl>
      )}
      {source && <SourceIssueChip source={source} />}
      {spec.acceptance_criteria && spec.acceptance_criteria.length > 0 && (
        <div className="text-sm">
          <p className="text-xs font-medium">{t("orchestration:criteriaHeading")}</p>
          <ul className="list-disc pl-5">
            {spec.acceptance_criteria.map((criterion) => (
              <li key={criterion} className="break-words">
                {criterion}
              </li>
            ))}
          </ul>
        </div>
      )}
    </>
  );
}

/** A coordinator's task proposal rendered in place of its chat comment. */
export function ProposalCard({ comment, proposal, unavailable, ...rest }: CardProps) {
  const { t } = useTranslation();
  if (!proposal)
    return (
      <article className="rounded-lg border bg-card p-3 my-3" data-testid="proposal-card">
        <MarkdownComment content={comment.content} />
        {unavailable && (
          <p className="text-xs text-muted-foreground">{t("orchestration:proposalUnavailable")}</p>
        )}
      </article>
    );
  return <LoadedProposalCard proposal={proposal} {...rest} />;
}

function LoadedProposalCard({
  proposal,
  ...rest
}: Omit<CardProps, "comment" | "unavailable"> & { proposal: TaskProposal }) {
  const { t } = useTranslation();
  const headingId = useId();
  const card = useRef<HTMLElement>(null);
  const spec = proposal.final_spec ?? proposal.spec;
  return (
    <article
      ref={card}
      tabIndex={-1}
      aria-labelledby={headingId}
      className="my-3 space-y-2 rounded-lg border bg-card p-3 shadow-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
      data-testid="proposal-card"
      data-proposal-id={proposal.id}
      data-status={proposal.status}
    >
      <header className="flex flex-wrap items-center gap-2">
        <span className="text-xs font-medium text-muted-foreground">
          {t("orchestration:proposalTitle")}
        </span>
        <Badge variant={proposal.status === "pending" ? "default" : "secondary"}>
          {t(STATUS_KEYS[proposal.status])}
        </Badge>
      </header>
      <h3 id={headingId} className="break-words font-semibold">
        {spec.title}
      </h3>
      <SpecDetails spec={spec} />
      <ProposalActions proposal={proposal} {...rest} onDecided={() => card.current?.focus()} />
    </article>
  );
}
