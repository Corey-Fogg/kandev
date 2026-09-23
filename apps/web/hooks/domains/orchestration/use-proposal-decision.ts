import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { ApiError } from "@/lib/api/client";
import {
  approveTaskProposal,
  dismissTaskProposal,
  getTaskProposal,
  type ProposalEdits,
  type TaskProposal,
} from "@/lib/api/domains/orchestration-proposals-api";

/** Backend error codes and messages with a translated explanation. */
// i18n-exempt: backend wire error texts matched against a response, never rendered.
const KNOWN_ERRORS: { match: (text: string) => boolean; key: string }[] = [
  {
    match: (text) => text === "proposal_already_decided",
    key: "orchestration:proposalErrorDecided",
  },
  {
    match: (text) => text === "proposal_approval_in_progress",
    key: "orchestration:proposalErrorInProgress",
  },
  {
    match: (text) => text === "title is required",
    key: "orchestration:proposalErrorTitleRequired",
  },
  {
    match: (text) => text.startsWith("task title is too long"),
    key: "orchestration:proposalTitleTooLong",
  },
  {
    match: (text) => text.startsWith("execution_mode must be"),
    key: "orchestration:proposalErrorInvalidMode",
  },
  {
    match: (text) => text.startsWith("reason must be at most"),
    key: "orchestration:proposalErrorReasonTooLong",
  },
];

function backendText(error: unknown) {
  if (error instanceof ApiError) {
    const body = error.body as { error?: unknown } | null;
    if (typeof body?.error === "string" && body.error) return body.error;
  }
  return error instanceof Error ? error.message : String(error);
}

/** A translated explanation of a known backend error; unknown text is shown as a fallback. */
export function proposalErrorText(t: TFunction, error: unknown) {
  const text = backendText(error);
  const known = KNOWN_ERRORS.find((entry) => entry.match(text));
  return known ? t(known.key) : t("orchestration:proposalDecisionFailed", { error: text });
}

function conflictProposal(error: unknown): TaskProposal | null {
  if (!(error instanceof ApiError) || error.status !== 409) return null;
  const body = error.body as { proposal?: TaskProposal } | null;
  return body?.proposal?.id ? body.proposal : null;
}

/**
 * Approves or dismisses one proposal. Only one decision request runs at a time,
 * so a double click sends one request; every returned proposal, including the
 * current one a 409 carries, is handed to `onChange`. A 409 also leaves a short
 * notice saying why the card changed, and any other failure re-reads the
 * proposal so a released approval shows as pending again.
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
  const [notice, setNotice] = useState("");
  const reread = () =>
    Promise.resolve()
      .then(() => getTaskProposal(workspaceId, orchestratorId, proposalId))
      .then(onChange)
      .catch(() => undefined);
  const fail = (e: unknown) => {
    const current = conflictProposal(e);
    if (current) {
      onChange(current);
      setNotice(proposalErrorText(t, e));
    } else if (e instanceof ApiError && e.status === 403) {
      setError(t("orchestration:proposalDecisionForbidden"));
    } else {
      setError(proposalErrorText(t, e));
      void reread();
    }
  };
  const run = async (request: () => Promise<{ proposal: TaskProposal }>) => {
    if (inflight.current) return false;
    inflight.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      onChange((await request()).proposal);
      return true;
    } catch (e) {
      fail(e);
      return false;
    } finally {
      inflight.current = false;
      setBusy(false);
    }
  };
  return {
    busy,
    error,
    notice,
    approve: (edits?: ProposalEdits) =>
      run(() => approveTaskProposal(workspaceId, orchestratorId, proposalId, edits)),
    dismiss: (reason?: string) =>
      run(() => dismissTaskProposal(workspaceId, orchestratorId, proposalId, reason)),
  };
}
