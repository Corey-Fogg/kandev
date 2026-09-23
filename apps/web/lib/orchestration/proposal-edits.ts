import type { ProposalEdits, ProposalSpec } from "@/lib/api/domains/orchestration-proposals-api";

export const PROPOSAL_TITLE_MAX = 60;
export const PROPOSAL_CRITERIA_MAX = 10;
export const PROPOSAL_CRITERION_MAX = 300;

/** The editable proposal fields, with absent values as empty strings. */
export type ProposalDraft = {
  title: string;
  description: string;
  workflow_id: string;
  workflow_step_id: string;
  repository_id: string;
  assignee: string;
  execution_mode: string;
  acceptance_criteria: string[];
};

const TEXT_FIELDS = [
  "title",
  "description",
  "workflow_id",
  "workflow_step_id",
  "repository_id",
  "assignee",
  "execution_mode",
] as const;

export function draftFromSpec(spec: ProposalSpec): ProposalDraft {
  return {
    title: spec.title,
    description: spec.description ?? "",
    workflow_id: spec.workflow_id ?? "",
    workflow_step_id: spec.workflow_step_id ?? "",
    repository_id: spec.repository_id ?? "",
    assignee: spec.assignee ?? "",
    execution_mode: spec.execution_mode ?? "",
    acceptance_criteria: [...(spec.acceptance_criteria ?? [])],
  };
}

const criteriaOf = (items: string[]) => items.map((item) => item.trim()).filter(Boolean);
const runes = (value: string) => [...value].length;

/** A human-approved title must fit without truncation. */
export function validDraft(draft: ProposalDraft) {
  const title = runes(draft.title.trim());
  return (
    title >= 1 &&
    title <= PROPOSAL_TITLE_MAX &&
    criteriaOf(draft.acceptance_criteria).length <= PROPOSAL_CRITERIA_MAX &&
    draft.acceptance_criteria.every((item) => runes(item.trim()) <= PROPOSAL_CRITERION_MAX)
  );
}

/** Only the fields the user changed; the backend keeps every absent field as proposed. */
export function proposalEdits(spec: ProposalSpec, draft: ProposalDraft): ProposalEdits {
  const original = draftFromSpec(spec);
  const edits: Record<string, unknown> = {};
  for (const field of TEXT_FIELDS) {
    const next = field === "title" ? draft.title.trim() : draft[field];
    if (next !== original[field]) edits[field] = next;
  }
  const criteria = criteriaOf(draft.acceptance_criteria);
  if (JSON.stringify(criteria) !== JSON.stringify(criteriaOf(original.acceptance_criteria)))
    edits.acceptance_criteria = criteria;
  return edits as ProposalEdits;
}
