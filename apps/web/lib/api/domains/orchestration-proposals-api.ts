import { fetchJson } from "../client";

export type ProposalSource = { tracker: "jira" | "linear"; key: string; url?: string };

/** A task the coordinator asked to create; empty optional fields are omitted. */
export type ProposalSpec = {
  title: string;
  description?: string;
  workflow_id?: string;
  workflow_step_id?: string;
  repository_id?: string;
  parent_id?: string;
  /** Execution profile id. */
  assignee?: string;
  execution_mode?: "execute" | "design";
  external_id?: string;
  source?: ProposalSource;
  acceptance_criteria?: string[];
};

export type ProposalStatus = "pending" | "approving" | "approved" | "dismissed";

export type TaskProposal = {
  id: string;
  orchestrator_id: string;
  workspace_id: string;
  conversation_task_id: string;
  status: ProposalStatus;
  spec: ProposalSpec;
  /** Present only when the proposal was approved with edits. */
  final_spec?: ProposalSpec | null;
  edited: boolean;
  task_id: string;
  duplicate: boolean;
  dismiss_reason: string;
  decided_by: string;
  created_at: string;
  decided_at: string | null;
};

/** Fields the user changed while approving; an absent field keeps the proposed value. */
export type ProposalEdits = Partial<
  Pick<
    ProposalSpec,
    | "title"
    | "description"
    | "workflow_id"
    | "workflow_step_id"
    | "repository_id"
    | "assignee"
    | "execution_mode"
    | "acceptance_criteria"
  >
>;

export type ProposalApproval = { proposal: TaskProposal; task_id: string; duplicate: boolean };

const proposals = (ws: string, id: string) =>
  `/api/v1/orchestration/workspaces/${encodeURIComponent(ws)}/orchestrators/${encodeURIComponent(id)}/proposals`;
const one = (ws: string, id: string, proposalId: string) =>
  `${proposals(ws, id)}/${encodeURIComponent(proposalId)}`;
const post = (body: unknown) => ({ init: { method: "POST", body: JSON.stringify(body) } });

export const listTaskProposals = (ws: string, id: string, status: "pending" | "all" = "all") =>
  fetchJson<{ proposals: TaskProposal[] }>(
    `${proposals(ws, id)}?${new URLSearchParams({ status, limit: "50" })}`,
  );

export const getTaskProposal = (ws: string, id: string, proposalId: string) =>
  fetchJson<TaskProposal>(one(ws, id, proposalId));

export const approveTaskProposal = (
  ws: string,
  id: string,
  proposalId: string,
  edits?: ProposalEdits,
) =>
  fetchJson<ProposalApproval>(
    `${one(ws, id, proposalId)}/approve`,
    post(edits && Object.keys(edits).length > 0 ? { edits } : {}),
  );

export const dismissTaskProposal = (ws: string, id: string, proposalId: string, reason?: string) =>
  fetchJson<{ proposal: TaskProposal }>(
    `${one(ws, id, proposalId)}/dismiss`,
    post(reason?.trim() ? { reason: reason.trim() } : {}),
  );
