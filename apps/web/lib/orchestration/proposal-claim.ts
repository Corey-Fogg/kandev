import type { TaskProposal } from "@/lib/api/domains/orchestration-proposals-api";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

/** Matches the backend's proposalClaimStaleAge: an older claim may be retried. */
export const PROPOSAL_CLAIM_STALE_MS = 5 * 60 * 1000;

/**
 * Whether an approving proposal's claim is old enough that the approval was
 * interrupted and may be retried. A claim with no readable time is stale, as
 * the backend treats it.
 */
export function approvalClaimStale(proposal: Pick<TaskProposal, "claimed_at">, nowMs: number) {
  const claimed = parseStrictRfc3339Timestamp(proposal.claimed_at ?? undefined);
  if (claimed === null) return true;
  return nowMs - Number(claimed / BigInt(1_000_000)) > PROPOSAL_CLAIM_STALE_MS;
}
