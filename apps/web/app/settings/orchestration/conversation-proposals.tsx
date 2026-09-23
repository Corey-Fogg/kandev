import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { CommentRenderer } from "@/components/task/simple/comment-renderer-context";
import { useConversationProposals } from "@/hooks/domains/orchestration/use-conversation-proposals";
import type { TaskComment } from "@/app/office/tasks/[id]/types";
import { ProposalCard } from "./proposal-card";

/** Renders proposal comments as decision cards and reports the ones awaiting a decision. */
export function useProposalRenderer(
  workspaceId: string,
  orchestratorId: string,
  comments: TaskComment[],
) {
  const proposals = useConversationProposals(workspaceId, orchestratorId, comments);
  const { byId, unavailable, replace } = proposals;
  const renderer = useCallback<CommentRenderer>(
    (comment) =>
      comment.source === "proposal" ? (
        <ProposalCard
          comment={comment}
          proposal={byId.get(comment.id)}
          unavailable={unavailable.has(comment.id)}
          workspaceId={workspaceId}
          orchestratorId={orchestratorId}
          onChange={replace}
        />
      ) : null,
    [byId, unavailable, replace, workspaceId, orchestratorId],
  );
  return {
    renderer,
    pendingCount: proposals.pendingCount,
    firstPendingId: proposals.firstPendingId,
  };
}

/** Jumps to the first proposal card that still needs a decision. */
export function ProposalsBanner({ count, targetId }: { count: number; targetId: string }) {
  const { t } = useTranslation();
  if (count === 0) return null;
  return (
    <Button
      type="button"
      variant="outline"
      className="mb-3 w-full cursor-pointer justify-start max-md:min-h-11"
      onClick={() =>
        document
          .getElementById(`comment-${targetId}`)
          ?.scrollIntoView({ behavior: "smooth", block: "center" })
      }
      data-testid="proposals-pending-banner"
    >
      {t("orchestration:proposalsPending", { count })}
    </Button>
  );
}
