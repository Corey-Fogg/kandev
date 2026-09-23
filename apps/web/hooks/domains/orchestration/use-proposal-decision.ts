import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "@/lib/api/client";
import {
  approveTaskProposal,
  dismissTaskProposal,
  type ProposalEdits,
  type TaskProposal,
} from "@/lib/api/domains/orchestration-proposals-api";

function errorText(error: unknown) {
  if (error instanceof ApiError) {
    const body = error.body as { error?: unknown } | null;
    if (typeof body?.error === "string" && body.error) return body.error;
  }
  return error instanceof Error ? error.message : String(error);
}

function decidedProposal(error: unknown): TaskProposal | null {
  if (!(error instanceof ApiError) || error.status !== 409) return null;
  const body = error.body as { proposal?: TaskProposal } | null;
  return body?.proposal?.id ? body.proposal : null;
}

/**
 * Approves or dismisses one proposal. Only one decision request runs at a time,
 * so a double click sends one request; every returned proposal, including the
 * current one a 409 carries, is handed to `onChange`.
 */
export function useProposalDecision(
  workspaceId: string,
  orchestratorId: string,
  proposalId: string,
  onChange: (proposal: TaskProposal) => void,
) {
  const { t } = useTranslation();
  const inflight = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const run = async (request: () => Promise<{ proposal: TaskProposal }>) => {
    if (inflight.current) return false;
    inflight.current = true;
    setBusy(true);
    setError("");
    try {
      onChange((await request()).proposal);
      return true;
    } catch (e) {
      const current = decidedProposal(e);
      if (current) onChange(current);
      else if (e instanceof ApiError && e.status === 403)
        setError(t("orchestration:proposalDecisionForbidden"));
      else setError(t("orchestration:proposalDecisionFailed", { error: errorText(e) }));
      return false;
    } finally {
      inflight.current = false;
      setBusy(false);
    }
  };
  return {
    busy,
    error,
    approve: (edits?: ProposalEdits) =>
      run(() => approveTaskProposal(workspaceId, orchestratorId, proposalId, edits)),
    dismiss: (reason?: string) =>
      run(() => dismissTaskProposal(workspaceId, orchestratorId, proposalId, reason)),
  };
}
