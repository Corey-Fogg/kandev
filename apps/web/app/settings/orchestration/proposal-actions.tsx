import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Spinner } from "@kandev/ui/spinner";
import Link from "@/components/routing/app-link";
import { useProposalDecision } from "@/hooks/domains/orchestration/use-proposal-decision";
import type { TaskProposal } from "@/lib/api/domains/orchestration-proposals-api";
import { approvalClaimStale } from "@/lib/orchestration/proposal-claim";
import { linkToTask } from "@/lib/links";
import { ProposalEditForm } from "./proposal-edit-form";

const CONTROL = "cursor-pointer max-md:min-h-11";
const APPROVE_ID = "proposal-approve";
type Mode = "view" | "edit" | "dismiss";

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

/**
 * Moves focus into a form when it opens and back to the button that opened it
 * when it closes, so keyboard and screen-reader users keep their place.
 */
function useModeFocus(mode: Mode) {
  const formRef = useRef<HTMLDivElement>(null);
  const triggers = useRef<Partial<Record<Mode, HTMLButtonElement | null>>>({});
  const previous = useRef<Mode>(mode);
  useEffect(() => {
    const from = previous.current;
    previous.current = mode;
    if (from === mode) return;
    if (mode === "view") triggers.current[from]?.focus();
    else formRef.current?.querySelector<HTMLElement>("input, textarea, select")?.focus();
  }, [mode]);
  const trigger = (target: Mode) => (element: HTMLButtonElement | null) => {
    triggers.current[target] = element;
  };
  return { formRef, trigger };
}

function ViewButtons({
  proposal,
  busy,
  onApprove,
  onMode,
  trigger,
}: {
  proposal: TaskProposal;
  busy: boolean;
  onApprove: () => void;
  onMode: (mode: Mode) => void;
  trigger: (target: Mode) => (element: HTMLButtonElement | null) => void;
}) {
  const { t } = useTranslation();
  if (proposal.status === "approving") {
    // Another approval is creating the task; offer a retry only once its claim is stale.
    if (busy || !approvalClaimStale(proposal, Date.now())) return null;
    return (
      <Button variant="outline" onClick={onApprove} className={CONTROL} data-testid={APPROVE_ID}>
        {t("orchestration:proposalRetryApproval")}
      </Button>
    );
  }
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button disabled={busy} onClick={onApprove} className={CONTROL} data-testid={APPROVE_ID}>
        {t("orchestration:proposalApprove")}
      </Button>
      <Button
        ref={trigger("edit")}
        variant="outline"
        onClick={() => onMode("edit")}
        className={CONTROL}
      >
        {t("orchestration:proposalEdit")}
      </Button>
      <Button
        ref={trigger("dismiss")}
        variant="ghost"
        onClick={() => onMode("dismiss")}
        className={CONTROL}
      >
        {t("orchestration:proposalDismiss")}
      </Button>
    </div>
  );
}

/** The decision controls of a proposal card, or its outcome once decided. */
export function ProposalActions({
  proposal,
  workspaceId,
  orchestratorId,
  onChange,
  onDecided,
}: {
  proposal: TaskProposal;
  workspaceId: string;
  orchestratorId: string;
  onChange: (proposal: TaskProposal) => void;
  /** Called after this card's own decision succeeds, to move focus to the card. */
  onDecided: () => void;
}) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<Mode>("view");
  const decision = useProposalDecision(workspaceId, orchestratorId, proposal.id, onChange);
  const { formRef, trigger } = useModeFocus(mode);
  const awaiting = proposal.status === "pending" || proposal.status === "approving";
  if (!awaiting) return <DecidedNotes proposal={proposal} />;
  const settle = (ok: boolean) => {
    if (!ok) return;
    setMode("view");
    onDecided();
  };
  const editable = mode !== "view" && proposal.status === "pending";
  return (
    <div className="space-y-2">
      <div ref={formRef}>
        {editable && mode === "edit" && (
          <ProposalEditForm
            workspaceId={workspaceId}
            spec={proposal.spec}
            busy={decision.busy}
            onSubmit={(edits) => void decision.approve(edits).then(settle)}
            onCancel={() => setMode("view")}
          />
        )}
        {editable && mode === "dismiss" && (
          <DismissForm
            busy={decision.busy}
            onConfirm={(reason) => void decision.dismiss(reason).then(settle)}
            onCancel={() => setMode("view")}
          />
        )}
      </div>
      {!editable && (
        <ViewButtons
          proposal={proposal}
          busy={decision.busy}
          onApprove={() => void decision.approve().then(settle)}
          onMode={setMode}
          trigger={trigger}
        />
      )}
      <div role="status" className="text-xs text-muted-foreground">
        {(decision.busy || proposal.status === "approving") && (
          <span className="inline-flex items-center gap-1">
            <Spinner aria-hidden="true" />
            {t("orchestration:proposalDeciding")}
          </span>
        )}
        {decision.notice && <span className="block">{decision.notice}</span>}
      </div>
      {decision.error && (
        <p role="alert" className="text-sm text-destructive">
          {decision.error}
        </p>
      )}
    </div>
  );
}
