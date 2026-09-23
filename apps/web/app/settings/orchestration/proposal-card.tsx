import { useContext, useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Spinner } from "@kandev/ui/spinner";
import Link from "@/components/routing/app-link";
import { MarkdownComment } from "@/components/task/simple/markdown-comment";
import { SourceIssueChip } from "@/app/coordinator/coordinator-task-signals";
import { useProposalDecision } from "@/hooks/domains/orchestration/use-proposal-decision";
import type { TaskComment } from "@/app/office/tasks/[id]/types";
import type { ProposalSpec, TaskProposal } from "@/lib/api/domains/orchestration-proposals-api";
import { safeHttpsUrl } from "@/lib/orchestration/coordinator-task-signals";
import { ProposalCatalogContext, proposalNames } from "@/lib/orchestration/proposal-catalog";
import { linkToTask } from "@/lib/links";
import { ProposalEditForm } from "./proposal-edit-form";

const CONTROL = "cursor-pointer max-md:min-h-11";
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

function DismissForm({
  busy,
  onConfirm,
  onCancel,
}: {
  busy: boolean;
  onConfirm: (reason: string) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [reason, setReason] = useState("");
  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={(event) => {
        event.preventDefault();
        onConfirm(reason);
      }}
    >
      <div className="min-w-0 flex-1 space-y-1">
        <label htmlFor={id} className="text-xs text-muted-foreground">
          {t("orchestration:proposalDismissReason")}
        </label>
        <Input
          id={id}
          value={reason}
          maxLength={500}
          onChange={(event) => setReason(event.target.value)}
          className="max-md:min-h-11"
          data-testid="proposal-dismiss-reason"
        />
      </div>
      <Button type="submit" variant="destructive" disabled={busy} className={CONTROL}>
        {t("orchestration:proposalConfirmDismiss")}
      </Button>
      <Button type="button" variant="ghost" onClick={onCancel} className={CONTROL}>
        {t("orchestration:proposalCancelEdit")}
      </Button>
    </form>
  );
}

function DecidedNotes({ proposal }: { proposal: TaskProposal }) {
  const { t } = useTranslation();
  if (proposal.status === "dismissed")
    return proposal.dismiss_reason ? (
      <p className="text-sm text-muted-foreground">
        {t("orchestration:proposalDismissedReason", { reason: proposal.dismiss_reason })}
      </p>
    ) : null;
  return (
    <div className="space-y-1 text-sm">
      {proposal.task_id && (
        <Link className="underline cursor-pointer" href={linkToTask(proposal.task_id)}>
          {t("orchestration:proposalOpenTask")}
        </Link>
      )}
      {proposal.edited && (
        <p className="text-muted-foreground">{t("orchestration:proposalEditedNote")}</p>
      )}
      {proposal.duplicate && (
        <p className="text-muted-foreground">{t("orchestration:proposalDuplicateNote")}</p>
      )}
    </div>
  );
}

function ProposalActions({
  proposal,
  workspaceId,
  orchestratorId,
  onChange,
}: Omit<CardProps, "comment" | "proposal" | "unavailable"> & { proposal: TaskProposal }) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<"view" | "edit" | "dismiss">("view");
  const decision = useProposalDecision(workspaceId, orchestratorId, proposal.id, onChange);
  const awaiting = proposal.status === "pending" || proposal.status === "approving";
  if (!awaiting) return <DecidedNotes proposal={proposal} />;
  const close = (ok: boolean) => ok && setMode("view");
  return (
    <div className="space-y-2">
      {mode === "edit" && (
        <ProposalEditForm
          workspaceId={workspaceId}
          spec={proposal.spec}
          busy={decision.busy}
          onSubmit={(edits) => void decision.approve(edits).then(close)}
          onCancel={() => setMode("view")}
        />
      )}
      {mode === "dismiss" && (
        <DismissForm
          busy={decision.busy}
          onConfirm={(reason) => void decision.dismiss(reason).then(close)}
          onCancel={() => setMode("view")}
        />
      )}
      {mode === "view" && (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            disabled={decision.busy}
            onClick={() => void decision.approve()}
            className={CONTROL}
            data-testid="proposal-approve"
          >
            {t("orchestration:proposalApprove")}
          </Button>
          {proposal.status === "pending" && (
            <>
              <Button variant="outline" onClick={() => setMode("edit")} className={CONTROL}>
                {t("orchestration:proposalEdit")}
              </Button>
              <Button variant="ghost" onClick={() => setMode("dismiss")} className={CONTROL}>
                {t("orchestration:proposalDismiss")}
              </Button>
            </>
          )}
        </div>
      )}
      <div role="status" className="text-xs text-muted-foreground">
        {(decision.busy || proposal.status === "approving") && (
          <span className="inline-flex items-center gap-1">
            <Spinner aria-hidden="true" />
            {t("orchestration:proposalDeciding")}
          </span>
        )}
      </div>
      {decision.error && (
        <p role="alert" className="text-sm text-destructive">
          {decision.error}
        </p>
      )}
    </div>
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
  const spec = proposal.final_spec ?? proposal.spec;
  return (
    <article
      className="my-3 space-y-2 rounded-lg border bg-card p-3 shadow-sm"
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
      <h3 className="break-words font-semibold">{spec.title}</h3>
      <SpecDetails spec={spec} />
      <ProposalActions proposal={proposal} {...rest} />
    </article>
  );
}
